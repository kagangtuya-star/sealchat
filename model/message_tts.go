package model

import "sealchat/protocol"

func (m *MessageModel) ValidTTS() *protocol.MessageTTS {
	if m == nil || m.IsRevoked || m.IsDeleted || m.DeletedAt != nil {
		return nil
	}
	if m.TTSData != nil && m.TTSData.MessageRevision == m.EditCount {
		return m.TTSData
	}
	if m.TTSStatus != "" {
		return &protocol.MessageTTS{Status: m.TTSStatus, MessageRevision: m.EditCount}
	}
	return nil
}
