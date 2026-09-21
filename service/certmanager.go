package service

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/caddyserver/certmagic"
	"github.com/mholt/acmez/v3/acme"
	"go.uber.org/zap"

	"sealchat/utils"
)

const (
	letEncryptIssuerEvent        = "certmagic"
	letsEncryptShortLivedProfile = "shortlived"
	certificateLogLimit          = 200
)

type CertificateManagerOptions struct {
	SkipObtain bool
}

type CertificateStatus struct {
	Enabled              bool       `json:"enabled"`
	RuntimeActive        bool       `json:"runtimeActive"`
	SubjectIP            string     `json:"subjectIp"`
	Issuer               string     `json:"issuer"`
	Challenge            string     `json:"challenge"`
	CertificatePresent   bool       `json:"certificatePresent"`
	NotBefore            *time.Time `json:"notBefore,omitempty"`
	NotAfter             *time.Time `json:"notAfter,omitempty"`
	RemainingDays        int        `json:"remainingDays"`
	LastError            string     `json:"lastError,omitempty"`
	LastCheckAt          *time.Time `json:"lastCheckAt,omitempty"`
	LastSuccessAt        *time.Time `json:"lastSuccessAt,omitempty"`
	NextCheckAt          *time.Time `json:"nextCheckAt,omitempty"`
	RetryCount           int        `json:"retryCount"`
	Retrying             bool       `json:"retrying"`
	RenewBeforeDays      int        `json:"renewBeforeDays"`
	CheckIntervalMinutes int        `json:"checkIntervalMinutes"`
	RetryInitialMinutes  int        `json:"retryInitialMinutes"`
	RetryMaxMinutes      int        `json:"retryMaxMinutes"`
}

type CertificateLogEntry struct {
	Time      time.Time `json:"time"`
	Level     string    `json:"level"`
	Event     string    `json:"event"`
	Message   string    `json:"message"`
	SubjectIP string    `json:"subjectIp,omitempty"`
	Issuer    string    `json:"issuer,omitempty"`
	Challenge string    `json:"challenge,omitempty"`
}

type CertificateManager struct {
	enabled      bool
	certConfig   *certmagic.Config
	cache        *certmagic.Cache
	acmeIssuer   *certmagic.ACMEIssuer
	zeroIssuer   *certmagic.ZeroSSLIssuer
	subjectIP    string
	issuer       utils.CertificateIssuer
	challenge    utils.CertificateChallenge
	lastError    string
	runtimeError string
	logs         []CertificateLogEntry
	logsMu       sync.Mutex
	stateMu      sync.Mutex
	obtainMu     sync.Mutex

	lastCheckAt          *time.Time
	lastSuccessAt        *time.Time
	nextCheckAt          *time.Time
	retryCount           int
	retrying             bool
	certificateReady     bool
	certificateNotBefore time.Time
	certificateNotAfter  time.Time
	renewBeforeDays      int
	checkIntervalMinutes int
	retryInitialMinutes  int
	retryMaxMinutes      int
	renewalLoopCancel    context.CancelFunc
	renewalLoopRunner    func(context.Context) time.Duration
}

func NewCertificateManager(ctx context.Context, appCfg *utils.AppConfig) (*CertificateManager, error) {
	return NewCertificateManagerWithOptions(ctx, appCfg, CertificateManagerOptions{})
}

func NewCertificateManagerWithOptions(ctx context.Context, appCfg *utils.AppConfig, opts CertificateManagerOptions) (*CertificateManager, error) {
	manager := &CertificateManager{}
	if appCfg == nil || !appCfg.Certificate.Enabled {
		return manager, nil
	}

	cfg := utils.NormalizeCertificateConfig(appCfg.Certificate)
	if err := utils.ValidateCertificateConfig(cfg); err != nil {
		return nil, err
	}

	logger := zap.NewNop()
	storage := &certmagic.FileStorage{Path: cfg.StorageDir}
	var activeCertConfig atomic.Pointer[certmagic.Config]
	cache := certmagic.NewCache(certmagic.CacheOptions{
		GetConfigForCert: func(cert certmagic.Certificate) (*certmagic.Config, error) {
			cmCfg := activeCertConfig.Load()
			if cmCfg == nil {
				return nil, fmt.Errorf("证书配置尚未初始化")
			}
			return cmCfg, nil
		},
		Logger: logger,
	})

	manager.enabled = true
	manager.cache = cache
	manager.subjectIP = cfg.SubjectIP
	manager.issuer = cfg.Issuer
	manager.challenge = cfg.Challenge
	manager.renewBeforeDays = cfg.RenewBeforeDays
	manager.checkIntervalMinutes = cfg.CheckIntervalMinutes
	manager.retryInitialMinutes = cfg.RetryInitialMinutes
	manager.retryMaxMinutes = cfg.RetryMaxMinutes

	cmCfg := certmagic.New(cache, certmagic.Config{
		Storage:           storage,
		Logger:            logger,
		DefaultServerName: cfg.SubjectIP,
		OnEvent: func(ctx context.Context, event string, data map[string]any) error {
			manager.addLog("info", event, fmt.Sprintf("%s: %v", event, data))
			return nil
		},
	})
	issuer, acmeIssuer, zeroIssuer := manager.buildIssuer(cmCfg, storage, cfg, logger)
	cmCfg.Issuers = []certmagic.Issuer{issuer}
	activeCertConfig.Store(cmCfg)
	manager.certConfig = cmCfg
	manager.acmeIssuer = acmeIssuer
	manager.zeroIssuer = zeroIssuer

	manager.addLog("info", letEncryptIssuerEvent, "证书管理器已初始化")
	manager.loadExistingCertificate(ctx)
	if !opts.SkipObtain {
		if err := manager.CheckNow(ctx); err != nil {
			return nil, err
		}
	}
	return manager, nil
}

func (m *CertificateManager) buildIssuer(cmCfg *certmagic.Config, storage certmagic.Storage, cfg utils.CertificateConfig, logger *zap.Logger) (certmagic.Issuer, *certmagic.ACMEIssuer, *certmagic.ZeroSSLIssuer) {
	if shouldUseZeroSSLAPIKeyIssuer(cfg) {
		issuer := &certmagic.ZeroSSLIssuer{
			APIKey:       cfg.ZeroSSLAPIKey,
			Storage:      storage,
			ValidityDays: 90,
			Logger:       logger,
		}
		return issuer, nil, issuer
	}

	template := certmagic.ACMEIssuer{
		Email:  cfg.Email,
		Agreed: true,
		Logger: logger,
	}
	switch cfg.Issuer {
	case utils.CertificateIssuerZeroSSL90Days:
		template.CA = certmagic.ZeroSSLProductionCA
		template.ExternalAccount = &acme.EAB{
			KeyID:  cfg.ZeroSSLEABKeyID,
			MACKey: cfg.ZeroSSLEABMACKey,
		}
	default:
		if cfg.Staging {
			template.CA = certmagic.LetsEncryptStagingCA
		} else {
			template.CA = certmagic.LetsEncryptProductionCA
		}
		template.TestCA = certmagic.LetsEncryptStagingCA
		template.Profile = letsEncryptShortLivedProfile
	}
	if cfg.Challenge == utils.CertificateChallengeHTTP01 {
		template.DisableTLSALPNChallenge = true
	} else {
		template.DisableHTTPChallenge = true
	}

	issuer := certmagic.NewACMEIssuer(cmCfg, template)
	return issuer, issuer, nil
}

func shouldUseZeroSSLAPIKeyIssuer(cfg utils.CertificateConfig) bool {
	hasCompleteEAB := cfg.ZeroSSLEABKeyID != "" && cfg.ZeroSSLEABMACKey != ""
	return cfg.Issuer == utils.CertificateIssuerZeroSSL90Days &&
		!hasCompleteEAB && cfg.ZeroSSLAPIKey != "" &&
		cfg.Challenge == utils.CertificateChallengeHTTP01
}

func (m *CertificateManager) Stop() {
	if m == nil {
		return
	}
	m.stateMu.Lock()
	cancel := m.renewalLoopCancel
	m.renewalLoopCancel = nil
	m.stateMu.Unlock()
	if cancel != nil {
		cancel()
	}
	if m.cache != nil {
		m.cache.Stop()
	}
}

func (m *CertificateManager) TLSConfig() *tls.Config {
	if m == nil || !m.enabled || m.certConfig == nil {
		return nil
	}
	tlsConfig := m.certConfig.TLSConfig()
	tlsConfig.NextProtos = normalizeCertificateNextProtos(tlsConfig.NextProtos)
	return tlsConfig
}

func normalizeCertificateNextProtos(items []string) []string {
	out := []string{"http/1.1"}
	seen := map[string]bool{"http/1.1": true}
	for _, item := range items {
		if item == "" || item == "h2" || seen[item] {
			continue
		}
		seen[item] = true
		out = append(out, item)
	}
	return out
}

func (m *CertificateManager) ObtainNow(ctx context.Context) error {
	return m.CheckNow(ctx)
}

func (m *CertificateManager) CheckNow(ctx context.Context) error {
	return m.runManualCertificateOperation(ctx, false)
}

func (m *CertificateManager) ForceRenewNow(ctx context.Context) error {
	return m.runManualCertificateOperation(ctx, true)
}

func (m *CertificateManager) runManualCertificateOperation(ctx context.Context, force bool) error {
	if m == nil || !m.enabled || m.certConfig == nil || m.subjectIP == "" {
		return fmt.Errorf("证书管理器未启用")
	}
	now := time.Now()
	_, err := m.ensureManagedCertificate(ctx, force)
	m.recordManualOperation(now, err)
	return err
}

func (m *CertificateManager) IsCertificateReady() bool {
	if m == nil {
		return false
	}
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	if !m.certificateReady {
		return false
	}
	now := time.Now()
	return (m.certificateNotBefore.IsZero() || !now.Before(m.certificateNotBefore)) &&
		(m.certificateNotAfter.IsZero() || now.Before(m.certificateNotAfter))
}

func (m *CertificateManager) ReportRuntimeError(event string, err error) {
	if m == nil || err == nil {
		return
	}
	m.stateMu.Lock()
	m.runtimeError = err.Error()
	m.stateMu.Unlock()
	m.addLog("error", event, err.Error())
}

func (m *CertificateManager) HTTPValidationHandler(next http.Handler) http.Handler {
	if next == nil {
		next = http.NewServeMux()
	}
	if m == nil || !m.enabled {
		return next
	}
	if m.acmeIssuer != nil {
		return m.acmeIssuer.HTTPChallengeHandler(next)
	}
	if m.zeroIssuer != nil {
		return m.zeroIssuer.HTTPValidationHandler(next)
	}
	return next
}

func (m *CertificateManager) Status(ctx context.Context) CertificateStatus {
	if m == nil {
		return CertificateStatus{}
	}
	m.stateMu.Lock()
	lastError := m.lastError
	if m.runtimeError != "" {
		lastError = m.runtimeError
	}
	status := CertificateStatus{
		Enabled:              m.enabled,
		RuntimeActive:        m.enabled && m.certConfig != nil,
		SubjectIP:            m.subjectIP,
		Issuer:               string(m.issuer),
		Challenge:            string(m.challenge),
		LastError:            lastError,
		LastCheckAt:          m.lastCheckAt,
		LastSuccessAt:        m.lastSuccessAt,
		NextCheckAt:          m.nextCheckAt,
		RetryCount:           m.retryCount,
		Retrying:             m.retrying,
		RenewBeforeDays:      m.renewBeforeDays,
		CheckIntervalMinutes: m.checkIntervalMinutes,
		RetryInitialMinutes:  m.retryInitialMinutes,
		RetryMaxMinutes:      m.retryMaxMinutes,
	}
	if !m.certificateNotAfter.IsZero() {
		notBefore := m.certificateNotBefore
		notAfter := m.certificateNotAfter
		status.CertificatePresent = true
		status.NotBefore = &notBefore
		status.NotAfter = &notAfter
		if time.Now().Before(notAfter) {
			status.RemainingDays = int(time.Until(notAfter).Hours() / 24)
		}
	}
	m.stateMu.Unlock()
	if !status.RuntimeActive {
		return status
	}
	cert, present, err := m.loadManagedCertificate(ctx)
	if err != nil || !present {
		return status
	}
	status.CertificatePresent = true
	notBefore := cert.Leaf.NotBefore
	notAfter := cert.Leaf.NotAfter
	status.NotBefore = &notBefore
	status.NotAfter = &notAfter
	if time.Now().Before(notAfter) {
		status.RemainingDays = int(time.Until(notAfter).Hours() / 24)
	}
	return status
}

func (m *CertificateManager) Logs(limit int) []CertificateLogEntry {
	if m == nil {
		return nil
	}
	m.logsMu.Lock()
	defer m.logsMu.Unlock()
	if limit <= 0 || limit > certificateLogLimit {
		limit = certificateLogLimit
	}
	if len(m.logs) <= limit {
		out := make([]CertificateLogEntry, len(m.logs))
		copy(out, m.logs)
		return out
	}
	out := make([]CertificateLogEntry, limit)
	copy(out, m.logs[len(m.logs)-limit:])
	return out
}

func (m *CertificateManager) addLog(level, event, message string) {
	entry := CertificateLogEntry{
		Time:      time.Now(),
		Level:     level,
		Event:     event,
		Message:   message,
		SubjectIP: m.subjectIP,
		Issuer:    string(m.issuer),
		Challenge: string(m.challenge),
	}
	log.Printf("[证书] [%s] %s: %s", level, event, message)
	m.logsMu.Lock()
	defer m.logsMu.Unlock()
	m.logs = append(m.logs, entry)
	if len(m.logs) > certificateLogLimit {
		m.logs = m.logs[len(m.logs)-certificateLogLimit:]
	}
}

func (m *CertificateManager) StartRenewalLoop(parent context.Context) {
	if m == nil || !m.enabled || m.certConfig == nil || m.subjectIP == "" {
		return
	}
	m.stateMu.Lock()
	if m.renewalLoopCancel != nil {
		m.stateMu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(parent)
	m.renewalLoopCancel = cancel
	runner := m.renewalLoopRunner
	m.stateMu.Unlock()

	if runner == nil {
		runner = m.runSingleRenewalPass
	}
	m.addLog("info", "renewal", "自动续期守护已启动")
	go m.runRenewalLoop(ctx, runner)
}

func (m *CertificateManager) runRenewalLoop(ctx context.Context, runner func(context.Context) time.Duration) {
	for {
		delay := runner(ctx)
		if delay <= 0 {
			delay = time.Duration(m.checkIntervalMinutes) * time.Minute
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			m.addLog("info", "renewal", "自动续期守护已停止")
			return
		case <-timer.C:
		}
	}
}

func (m *CertificateManager) runSingleRenewalPass(ctx context.Context) time.Duration {
	now := time.Now()
	action, err := m.ensureManagedCertificate(ctx, false)
	if err != nil {
		delay := nextCertificateCheckDelay(true, m.currentRetryCount()+1, m.checkIntervalMinutes, m.retryInitialMinutes, m.retryMaxMinutes)
		m.recordRenewalFailure(now, delay, err)
		m.addLog("error", "renewal", fmt.Sprintf("自动续期守护检查失败，将在 %s 后重试: %v", delay, err))
		return delay
	}

	delay := nextCertificateCheckDelay(false, 0, m.checkIntervalMinutes, m.retryInitialMinutes, m.retryMaxMinutes)
	m.recordRenewalSuccess(now, delay)
	if action == certificateActionNone {
		m.addLog("info", "renewal", "证书剩余时间充足，本轮无需续期")
	}
	return delay
}

func (m *CertificateManager) currentRetryCount() int {
	if m == nil {
		return 0
	}
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	return m.retryCount
}

type certificateAction uint8

const (
	certificateActionNone certificateAction = iota
	certificateActionObtain
	certificateActionRenew
)

func selectCertificateAction(present bool, notAfter, now time.Time, thresholdDays int, force bool) certificateAction {
	if !present {
		return certificateActionObtain
	}
	if force || notAfter.Sub(now) <= time.Duration(thresholdDays)*24*time.Hour {
		return certificateActionRenew
	}
	return certificateActionNone
}

func nextCertificateCheckDelay(retrying bool, retryCount int, normalMinutes int, initialRetryMinutes int, maxRetryMinutes int) time.Duration {
	if !retrying || retryCount <= 0 {
		return time.Duration(normalMinutes) * time.Minute
	}
	delay := initialRetryMinutes
	for i := 1; i < retryCount; i++ {
		delay *= 2
		if delay >= maxRetryMinutes {
			delay = maxRetryMinutes
			break
		}
	}
	return time.Duration(delay) * time.Minute
}

func (m *CertificateManager) recordRenewalFailure(now time.Time, nextDelay time.Duration, err error) {
	if m == nil {
		return
	}
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	m.lastError = err.Error()
	m.retryCount++
	m.retrying = true
	lastCheckAt := now
	nextCheckAt := now.Add(nextDelay)
	m.lastCheckAt = &lastCheckAt
	m.nextCheckAt = &nextCheckAt
}

func (m *CertificateManager) recordRenewalSuccess(now time.Time, nextDelay time.Duration) {
	if m == nil {
		return
	}
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	m.lastError = ""
	m.retryCount = 0
	m.retrying = false
	lastCheckAt := now
	lastSuccessAt := now
	nextCheckAt := now.Add(nextDelay)
	m.lastCheckAt = &lastCheckAt
	m.lastSuccessAt = &lastSuccessAt
	m.nextCheckAt = &nextCheckAt
}

func (m *CertificateManager) recordManualOperation(now time.Time, err error) {
	m.stateMu.Lock()
	defer m.stateMu.Unlock()
	lastCheckAt := now
	m.lastCheckAt = &lastCheckAt
	if err != nil {
		m.lastError = err.Error()
		return
	}
	lastSuccessAt := now
	m.lastSuccessAt = &lastSuccessAt
	m.lastError = ""
	m.retrying = false
	m.retryCount = 0
}

func (m *CertificateManager) loadExistingCertificate(ctx context.Context) {
	cert, present, err := m.loadManagedCertificate(ctx)
	if err != nil {
		m.stateMu.Lock()
		m.lastError = err.Error()
		m.stateMu.Unlock()
		m.addLog("error", "load", err.Error())
		return
	}
	if !present || cert.Leaf == nil || !certificateIsUsable(cert.Leaf.NotBefore, cert.Leaf.NotAfter, time.Now()) {
		return
	}
	m.setCertificateReadyFromCert(cert)
	m.addLog("info", "load", "已加载磁盘中的现有证书")
}

func (m *CertificateManager) loadManagedCertificate(ctx context.Context) (certmagic.Certificate, bool, error) {
	if m == nil || m.certConfig == nil || m.subjectIP == "" {
		return certmagic.Certificate{}, false, fmt.Errorf("证书管理器未启用")
	}
	cert, err := m.certConfig.CacheManagedCertificate(ctx, m.subjectIP)
	if errors.Is(err, fs.ErrNotExist) {
		if m.hasAnyManagedCertificateResource(ctx) {
			return certmagic.Certificate{}, false, fmt.Errorf("读取运行时证书失败: 证书存储资源不完整: %w", err)
		}
		return certmagic.Certificate{}, false, nil
	}
	if err != nil {
		return certmagic.Certificate{}, false, fmt.Errorf("读取运行时证书失败: %w", err)
	}
	if cert.Leaf == nil {
		return certmagic.Certificate{}, false, fmt.Errorf("读取运行时证书失败: 证书缺少 Leaf")
	}
	return cert, true, nil
}

func (m *CertificateManager) hasAnyManagedCertificateResource(ctx context.Context) bool {
	for _, issuer := range m.certConfig.Issuers {
		issuerKey := issuer.IssuerKey()
		if m.certConfig.Storage.Exists(ctx, certmagic.StorageKeys.SiteCert(issuerKey, m.subjectIP)) ||
			m.certConfig.Storage.Exists(ctx, certmagic.StorageKeys.SitePrivateKey(issuerKey, m.subjectIP)) ||
			m.certConfig.Storage.Exists(ctx, certmagic.StorageKeys.SiteMeta(issuerKey, m.subjectIP)) {
			return true
		}
	}
	return false
}

func certificateNotAfter(cert certmagic.Certificate) time.Time {
	if cert.Leaf == nil {
		return time.Time{}
	}
	return cert.Leaf.NotAfter
}

func certificateIsUsable(notBefore, notAfter, now time.Time) bool {
	return !now.Before(notBefore) && now.Before(notAfter)
}

func (m *CertificateManager) setCertificateReadyFromCert(cert certmagic.Certificate) {
	if cert.Leaf == nil {
		return
	}
	m.stateMu.Lock()
	m.certificateReady = certificateIsUsable(cert.Leaf.NotBefore, cert.Leaf.NotAfter, time.Now())
	m.certificateNotBefore = cert.Leaf.NotBefore
	m.certificateNotAfter = cert.Leaf.NotAfter
	m.stateMu.Unlock()
}

func (m *CertificateManager) ensureManagedCertificate(ctx context.Context, force bool) (certificateAction, error) {
	m.obtainMu.Lock()
	defer m.obtainMu.Unlock()
	cert, present, err := m.loadManagedCertificate(ctx)
	if err != nil {
		return certificateActionNone, err
	}
	if present && certificateIsUsable(cert.Leaf.NotBefore, cert.Leaf.NotAfter, time.Now()) {
		m.setCertificateReadyFromCert(cert)
	}
	action := selectCertificateAction(present, certificateNotAfter(cert), time.Now(), m.renewBeforeDays, force)
	return action, m.executeCertificateAction(ctx, action, cert)
}

func (m *CertificateManager) executeCertificateAction(ctx context.Context, action certificateAction, oldCert certmagic.Certificate) error {
	switch action {
	case certificateActionNone:
		m.addLog("info", "check", "证书剩余时间充足，无需续期")
		return nil
	case certificateActionObtain:
		m.addLog("info", "obtain", "开始申请证书")
		if err := m.certConfig.ManageSync(ctx, []string{m.subjectIP}); err != nil {
			m.addLog("error", "obtain", err.Error())
			return err
		}
		cert, present, err := m.loadManagedCertificate(ctx)
		if err != nil || !present || !certificateIsUsable(cert.Leaf.NotBefore, cert.Leaf.NotAfter, time.Now()) {
			if err != nil {
				return fmt.Errorf("证书申请完成，但加载运行时证书失败: %w", err)
			}
			return fmt.Errorf("证书申请完成，但加载运行时证书失败")
		}
		m.setCertificateReadyFromCert(cert)
		m.addLog("info", "obtain", "证书申请完成并已进入 CertMagic 管理")
		return nil
	case certificateActionRenew:
		m.addLog("info", "renew", "开始重新签发证书")
		if err := m.certConfig.RenewCertSync(ctx, m.subjectIP, true); err != nil {
			m.addLog("error", "renew", err.Error())
			return err
		}
		if _, err := m.reloadRenewedCertificate(ctx, oldCert); err != nil {
			return err
		}
		m.addLog("info", "renew", "证书重新签发并加载完成")
		return nil
	default:
		return fmt.Errorf("未知的证书操作")
	}
}

func (m *CertificateManager) reloadRenewedCertificate(ctx context.Context, oldCert certmagic.Certificate) (certmagic.Certificate, error) {
	oldHash := oldCert.Hash()
	newCert, present, err := m.loadManagedCertificate(ctx)
	if err != nil || !present {
		if err != nil {
			return certmagic.Certificate{}, fmt.Errorf("证书已续期，但重新加载到运行时缓存失败: %w", err)
		}
		return certmagic.Certificate{}, fmt.Errorf("证书已续期，但重新加载到运行时缓存失败")
	}
	if !certificateIsUsable(newCert.Leaf.NotBefore, newCert.Leaf.NotAfter, time.Now()) {
		if newCert.Hash() != "" && newCert.Hash() != oldHash {
			m.cache.Remove([]string{newCert.Hash()})
		}
		return certmagic.Certificate{}, fmt.Errorf("证书已续期，但新证书当前不可用")
	}

	staleHashes := staleCertificateHashes(m.subjectIP, newCert.Hash(), m.cache.AllMatchingCertificates(m.subjectIP))
	removeStaleCertificates(m.cache, staleHashes)
	m.setCertificateReadyFromCert(newCert)
	return newCert, nil
}

func removeStaleCertificates(cache *certmagic.Cache, hashes []string) {
	if cache != nil && len(hashes) > 0 {
		cache.Remove(hashes)
	}
}

func staleCertificateHashes(subject, newHash string, certs []certmagic.Certificate) []string {
	var hashes []string
	for _, cert := range certs {
		if cert.Hash() == "" || cert.Hash() == newHash || !certificateHasSubject(cert, subject) {
			continue
		}
		hashes = append(hashes, cert.Hash())
	}
	return hashes
}

func certificateHasSubject(cert certmagic.Certificate, subject string) bool {
	for _, name := range cert.Names {
		if name == subject {
			return true
		}
	}
	return false
}
