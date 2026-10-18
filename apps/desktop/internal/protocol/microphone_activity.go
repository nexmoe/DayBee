package protocol

type MicrophoneUsage struct {
	ID        string  `json:"id"`
	AppID     string  `json:"appId"`
	AppName   string  `json:"appName"`
	AppIcon   string  `json:"appIcon"`
	StartedAt string  `json:"startedAt"`
	EndedAt   *string `json:"endedAt"`
}

type MicrophoneActivitySnapshot struct {
	Supported bool              `json:"supported"`
	Error     *string           `json:"error"`
	Items     []MicrophoneUsage `json:"items"`
}
