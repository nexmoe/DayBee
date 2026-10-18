package permissions

import (
	"github.com/nexmoe/eve/apps/desktop/internal/protocol"
	"golang.org/x/sys/windows/registry"
)

func nativeStatus() protocol.MicrophonePermissionState {
	path := `Software\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\microphone`
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, candidate := range []string{path, path + `\NonPackaged`} {
			key, err := registry.OpenKey(root, candidate, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			value, _, err := key.GetStringValue("Value")
			key.Close()
			if err == nil && value == "Deny" {
				return protocol.PermissionDenied
			}
		}
	}
	return protocol.PermissionAuthorized
}
func nativeRequest() {}
