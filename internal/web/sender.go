package web

// SenderAddress は選べる送り先。
type SenderAddress struct {
	Kind    string         `json:"kind"`
	Address string         `json:"address"`
	Cameras []SenderCamera `json:"cameras"`
}
type SenderProtocol struct {
	URL      string `json:"url"`
	Address  string `json:"address"`
	StreamID string `json:"streamid,omitempty"`
	Key      string `json:"key,omitempty"`
	User     string `json:"user"`
	Password string `json:"password"`
}
type SenderCamera struct {
	ID   string          `json:"id"`
	Name string          `json:"name"`
	SRT  *SenderProtocol `json:"srt,omitempty"`
	RTMP *SenderProtocol `json:"rtmp,omitempty"`
	OBS  string          `json:"obs"`
}
type SenderSettings struct {
	Addresses []SenderAddress `json:"addresses"`
	Default   string          `json:"default"`
}
