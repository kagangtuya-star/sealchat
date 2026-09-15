package api

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"

	"sealchat/service"
	"sealchat/utils"
)

var runtimeCertificateManager *service.CertificateManager
var runtimeCertificateStateMu sync.RWMutex
var runtimeCertificateConfig *utils.CertificateConfig
var (
	startHTTP01CompanionForServing    = startHTTP01Companion
	startTLSALPN01CompanionForServing = startTLSALPN01Companion
)

func serveAppWithOptionalCertificate(app *fiber.App, config *utils.AppConfig) error {
	if config == nil || !config.Certificate.Enabled {
		return serveFiberHTTP(app, config)
	}

	manager, err := newCertificateManagerForServing(context.Background(), config)
	if err != nil {
		return fmt.Errorf("初始化证书管理器失败: %w", err)
	}
	setRuntimeCertificateManager(manager, config.Certificate)

	listenAddr := certificateBusinessListenAddr(config)
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		StopRuntimeCertificateManager()
		return fmt.Errorf("HTTPS 业务端口监听失败 %s: %w", listenAddr, err)
	}
	tlsConfig := manager.TLSConfig()
	if tlsConfig == nil {
		StopRuntimeCertificateManager()
		_ = ln.Close()
		return fmt.Errorf("证书 TLS 配置不可用")
	}
	for _, companionErr := range startCertificateChallengeCompanions(config, manager, tlsConfig, listenAddr) {
		manager.ReportRuntimeError("challenge", companionErr)
	}

	log.Printf("HTTPS listening at %s", listenAddr)
	if certificateListenAddrSharesHTTP(config, listenAddr) {
		httpListener, tlsListener := newCertificateProtocolMuxListeners(ln.Addr())
		go serveCertificateProtocolMux(ln, httpListener, tlsListener)
		go func() {
			if err := app.Listener(tls.NewListener(tlsListener, tlsConfig)); err != nil {
				log.Printf("HTTPS listener stopped: %v", err)
			}
		}()
		startCertificateMaintenance(manager)
		return app.Listener(httpListener)
	}

	go func() {
		if err := app.Listener(tls.NewListener(ln, tlsConfig)); err != nil {
			log.Printf("HTTPS listener stopped: %v", err)
		}
	}()
	startCertificateMaintenance(manager)
	return serveFiberHTTP(app, config)
}

func newCertificateManagerForServing(ctx context.Context, config *utils.AppConfig) (*service.CertificateManager, error) {
	return service.NewCertificateManagerWithOptions(ctx, config, service.CertificateManagerOptions{
		SkipObtain: true,
	})
}

func startCertificateMaintenance(manager *service.CertificateManager) {
	if manager == nil {
		return
	}
	manager.StartRenewalLoop(context.Background())
}

func StopRuntimeCertificateManager() {
	runtimeCertificateStateMu.Lock()
	manager := runtimeCertificateManager
	runtimeCertificateManager = nil
	runtimeCertificateConfig = nil
	runtimeCertificateStateMu.Unlock()
	if manager != nil {
		manager.Stop()
	}
}

func setRuntimeCertificateManager(manager *service.CertificateManager, cfg utils.CertificateConfig) {
	normalized := utils.NormalizeCertificateConfig(cfg)
	runtimeCertificateStateMu.Lock()
	runtimeCertificateManager = manager
	runtimeCertificateConfig = &normalized
	runtimeCertificateStateMu.Unlock()
}

func currentRuntimeCertificateManager() *service.CertificateManager {
	runtimeCertificateStateMu.RLock()
	defer runtimeCertificateStateMu.RUnlock()
	return runtimeCertificateManager
}

func certificateRuntimeRestartRequired() bool {
	cfg := utils.CertificateConfig{}
	if appConfig != nil {
		cfg = utils.NormalizeCertificateConfig(appConfig.Certificate)
	}
	runtimeCertificateStateMu.RLock()
	manager := runtimeCertificateManager
	snapshot := runtimeCertificateConfig
	runtimeCertificateStateMu.RUnlock()
	if manager == nil || snapshot == nil {
		return cfg.Enabled
	}
	return *snapshot != cfg
}

func startCertificateChallengeCompanions(config *utils.AppConfig, manager *service.CertificateManager, tlsConfig *tls.Config, listenAddr string) []error {
	var errs []error
	if config == nil {
		return errs
	}
	if config.Certificate.Challenge == utils.CertificateChallengeHTTP01 {
		if err := startHTTP01CompanionForServing(config, manager); err != nil {
			log.Printf("[证书] HTTP-01 companion 启动失败，业务端口仍继续启动: %v", err)
			errs = append(errs, err)
		}
	}
	if config.Certificate.Challenge == utils.CertificateChallengeTLSALPN01 && extractPort(listenAddr) != "443" {
		if err := startTLSALPN01CompanionForServing(config, tlsConfig); err != nil {
			log.Printf("[证书] TLS-ALPN-01 companion 启动失败，业务端口仍继续启动: %v", err)
			errs = append(errs, err)
		}
	}
	return errs
}

func serveFiberHTTP(app *fiber.App, config *utils.AppConfig) error {
	listenAddr := config.ServeAt
	if normalized, changed := utils.NormalizeServeAt(listenAddr); changed {
		listenAddr = normalized
		config.ServeAt = normalized
	}
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		host = ""
		port = "3212"
	}
	if port == "" {
		port = "3212"
	}
	mode := classifyListenMode(host)
	applyFallback := func(originalAddr, actualAddr string) {
		log.Printf("警告: 端口 %s 被占用，已切换到 %s", originalAddr, actualAddr)
		config.ServeAt = actualAddr
		newPort := extractPort(actualAddr)
		if newDomain, ok := updateDomainPort(config.Domain, newPort); ok {
			config.Domain = newDomain
		}
		utils.WriteConfig(config)
		log.Printf("配置文件已更新: serveAt=%s, domain=%s", config.ServeAt, config.Domain)
	}

	switch mode {
	case listenIPv6:
		actualAddr, usedFallback := utils.FindAvailablePortWithNetwork("tcp6", listenAddr)
		if usedFallback {
			applyFallback(listenAddr, actualAddr)
		}
		ln6, err := net.Listen("tcp6", actualAddr)
		if err != nil {
			return err
		}
		log.Printf("IPv6 listening at %s", actualAddr)
		return app.Listener(ln6)
	case listenDual:
		listenAddr4 := utils.FormatListenHostPort(host, port)
		actualAddr4, usedFallback := utils.FindAvailablePortWithNetwork("tcp4", listenAddr4)
		if usedFallback {
			applyFallback(listenAddr4, actualAddr4)
		}
		port = extractPort(actualAddr4)
		if port == "" {
			port = "3212"
		}
		listenAddr6 := utils.FormatListenHostPort("::", port)
		ln4, err4 := net.Listen("tcp4", actualAddr4)
		if err4 != nil {
			log.Printf("IPv4 listen failed: %v", err4)
		}
		ln6, err6 := net.Listen("tcp6", listenAddr6)
		if err6 != nil {
			log.Printf("IPv6 listen unavailable: %v", err6)
		} else {
			log.Printf("IPv6 listening at %s", listenAddr6)
		}
		if ln4 != nil {
			if ln6 != nil {
				go func() {
					if err := app.Listener(ln6); err != nil {
						log.Printf("IPv6 listener stopped: %v", err)
					}
				}()
			}
			log.Printf("IPv4 listening at %s", actualAddr4)
			return app.Listener(ln4)
		}
		if ln6 != nil {
			return app.Listener(ln6)
		}
		return err4
	default:
		actualAddr, usedFallback := utils.FindAvailablePortWithNetwork("tcp4", listenAddr)
		if usedFallback {
			applyFallback(listenAddr, actualAddr)
		}
		ln4, err := net.Listen("tcp4", actualAddr)
		if err != nil {
			return err
		}
		log.Printf("IPv4 listening at %s", actualAddr)
		return app.Listener(ln4)
	}
}

func startHTTP01Companion(config *utils.AppConfig, manager *service.CertificateManager) error {
	ln, err := net.Listen("tcp", ":80")
	if err != nil {
		return fmt.Errorf("HTTP-01 需要监听 80 端口，但启动失败: %w", err)
	}
	server := &http.Server{
		Handler: manager.HTTPValidationHandler(certificateRedirectHandler(config)),
	}
	go func() {
		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Printf("[证书] HTTP-01 companion stopped: %v", err)
		}
	}()
	log.Printf("[证书] HTTP-01 companion listening at :80")
	return nil
}

func startTLSALPN01Companion(config *utils.AppConfig, tlsConfig *tls.Config) error {
	ln, err := net.Listen("tcp", ":443")
	if err != nil {
		return fmt.Errorf("TLS-ALPN-01 需要监听 443 端口，但启动失败: %w", err)
	}
	server := &http.Server{
		Handler:   certificateRedirectHandler(config),
		TLSConfig: tlsConfig,
	}
	go func() {
		if err := server.Serve(tls.NewListener(ln, tlsConfig)); err != nil && err != http.ErrServerClosed {
			log.Printf("[证书] TLS-ALPN-01 companion stopped: %v", err)
		}
	}()
	log.Printf("[证书] TLS-ALPN-01 companion listening at :443")
	return nil
}

func certificateRedirectHandler(config *utils.AppConfig) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !shouldRedirectCertificateHTTP(config, r.Host, false) {
			http.NotFound(w, r)
			return
		}
		http.Redirect(w, r, certificateRedirectURL(config, r), http.StatusPermanentRedirect)
	})
}

func certificateHTTPRedirectMiddleware(config *utils.AppConfig) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if shouldRedirectCertificateHTTP(config, c.Hostname(), strings.EqualFold(c.Protocol(), "https")) {
			return c.Redirect(certificateRedirectTarget(config, c.OriginalURL()), fiber.StatusPermanentRedirect)
		}
		return c.Next()
	}
}

func shouldRedirectCertificateHTTP(config *utils.AppConfig, host string, isTLS bool) bool {
	if isTLS || config == nil || !config.Certificate.Enabled || !config.Certificate.ForceHTTPS || !config.Certificate.RedirectHTTP {
		return false
	}
	manager := currentRuntimeCertificateManager()
	if manager == nil || !manager.IsCertificateReady() {
		return false
	}
	return !isCertificateLoopbackHost(host)
}

func isCertificateLoopbackHost(host string) bool {
	hostname := strings.TrimSpace(host)
	if parsedHost, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = parsedHost
	}
	hostname = strings.Trim(hostname, "[]")
	hostname = strings.TrimSuffix(strings.ToLower(stripIPv6Zone(hostname)), ".")
	if hostname == "localhost" {
		return true
	}
	ip := net.ParseIP(hostname)
	return ip != nil && ip.IsLoopback()
}

func certificateBusinessListenAddr(config *utils.AppConfig) string {
	if config == nil {
		return ":3212"
	}
	if strings.TrimSpace(config.Certificate.HTTPSServeAt) != "" {
		return config.Certificate.HTTPSServeAt
	}
	return config.ServeAt
}

func certificateRedirectURL(config *utils.AppConfig, r *http.Request) string {
	return certificateRedirectTarget(config, r.URL.RequestURI())
}

func certificateRedirectTarget(config *utils.AppConfig, requestURI string) string {
	host := config.Certificate.SubjectIP
	_, port, err := net.SplitHostPort(certificateBusinessListenAddr(config))
	if err != nil || port == "" {
		port = "443"
	}
	redirectHost := utils.EnsureIPv6Bracket(host)
	if port != "443" {
		redirectHost = utils.FormatHostPort(host, port)
	}
	if requestURI == "" || !strings.HasPrefix(requestURI, "/") {
		requestURI = "/"
	}
	return "https://" + redirectHost + requestURI
}

func certificateListenAddrSharesHTTP(config *utils.AppConfig, listenAddr string) bool {
	if config == nil {
		return false
	}
	httpsHost, httpsPort := certificateListenHostPort(listenAddr)
	httpHost, httpPort := certificateListenHostPort(config.ServeAt)
	if httpsPort == "" || httpPort == "" || httpsPort != httpPort {
		return false
	}
	return httpsHost == httpHost || isWildcardListenHost(httpsHost) || isWildcardListenHost(httpHost)
}

func certificateListenHostPort(addr string) (string, string) {
	normalized, _ := utils.NormalizeServeAt(addr)
	host, port, err := net.SplitHostPort(normalized)
	if err != nil {
		return "", ""
	}
	host = strings.Trim(strings.ToLower(stripIPv6Zone(host)), "[]")
	return host, port
}

func isWildcardListenHost(host string) bool {
	return host == "" || host == "0.0.0.0" || host == "::"
}

type certificateMuxListener struct {
	addr      net.Addr
	ch        chan net.Conn
	done      chan struct{}
	closeOnce sync.Once
}

func newCertificateProtocolMuxListeners(addr net.Addr) (*certificateMuxListener, *certificateMuxListener) {
	return &certificateMuxListener{
			addr: addr,
			ch:   make(chan net.Conn, 64),
			done: make(chan struct{}),
		}, &certificateMuxListener{
			addr: addr,
			ch:   make(chan net.Conn, 64),
			done: make(chan struct{}),
		}
}

func (l *certificateMuxListener) Accept() (net.Conn, error) {
	select {
	case conn := <-l.ch:
		if conn == nil {
			return nil, net.ErrClosed
		}
		return conn, nil
	case <-l.done:
		return nil, net.ErrClosed
	}
}

func (l *certificateMuxListener) Close() error {
	l.closeOnce.Do(func() {
		close(l.done)
	})
	return nil
}

func (l *certificateMuxListener) Addr() net.Addr {
	return l.addr
}

func (l *certificateMuxListener) deliver(conn net.Conn) {
	select {
	case l.ch <- conn:
	case <-l.done:
		_ = conn.Close()
	}
}

func serveCertificateProtocolMux(base net.Listener, httpListener, tlsListener *certificateMuxListener) {
	for {
		conn, err := base.Accept()
		if err != nil {
			_ = httpListener.Close()
			_ = tlsListener.Close()
			if !errorsIsNetClosed(err) {
				log.Printf("certificate protocol mux stopped: %v", err)
			}
			return
		}
		go routeCertificateProtocolConn(conn, httpListener, tlsListener)
	}
}

func routeCertificateProtocolConn(conn net.Conn, httpListener, tlsListener *certificateMuxListener) {
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		_ = conn.Close()
		return
	}
	buffered := bufio.NewReader(conn)
	first, err := buffered.Peek(1)
	if err != nil {
		_ = conn.Close()
		return
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		_ = conn.Close()
		return
	}
	wrapped := &certificateBufferedConn{Conn: conn, reader: buffered}
	if len(first) > 0 && (first[0] == 0x16 || first[0] == 0x80) {
		tlsListener.deliver(wrapped)
		return
	}
	httpListener.deliver(wrapped)
}

type certificateBufferedConn struct {
	net.Conn
	reader *bufio.Reader
}

func (c *certificateBufferedConn) Read(p []byte) (int, error) {
	return c.reader.Read(p)
}

func errorsIsNetClosed(err error) bool {
	return err == net.ErrClosed || err == io.ErrClosedPipe || strings.Contains(err.Error(), "use of closed network connection")
}
