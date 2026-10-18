package permissions

import "github.com/nexmoe/DayBee/apps/desktop/internal/protocol"

func Status() protocol.MicrophonePermissionStatus {
	state := nativeStatus()
	message := "当前平台未提供完整的麦克风权限桥接。"
	switch state {
	case protocol.PermissionAuthorized:
		message = "麦克风权限已授权。"
	case protocol.PermissionDenied:
		message = "麦克风权限已拒绝，请到系统设置手动开启。"
	case protocol.PermissionRestricted:
		message = "麦克风权限受系统限制。"
	case protocol.PermissionNotDetermined:
		message = "麦克风权限尚未申请。"
	}
	return protocol.MicrophonePermissionStatus{Message: message, State: state, Supported: state != protocol.PermissionUnsupported}
}

func Request() protocol.MicrophonePermissionStatus { nativeRequest(); return Status() }
