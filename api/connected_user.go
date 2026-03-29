package api

import "strings"

func syncConnectedUserProfile(userID, nickname, avatar, nickColor string) {
	userID = strings.TrimSpace(userID)
	if userID == "" || userId2ConnInfoGlobal == nil {
		return
	}
	connMap, ok := userId2ConnInfoGlobal.Load(userID)
	if !ok || connMap == nil {
		return
	}
	connMap.Range(func(_ *WsSyncConn, info *ConnInfo) bool {
		if info == nil || info.User == nil {
			return true
		}
		if strings.TrimSpace(nickname) != "" {
			info.User.Nickname = nickname
		}
		info.User.Avatar = avatar
		info.User.NickColor = nickColor
		return true
	})
}
