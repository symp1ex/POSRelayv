package ws

type Message struct {
	Type       string                 `json:"type"`
	ClientID   string                 `json:"client_id,omitempty"`
	CommandID  string                 `json:"command_id,omitempty"`
	Command    string                 `json:"command,omitempty"`
	Prompt     string                 `json:"prompt,omitempty"`
	Result     map[string]interface{} `json:"result,omitempty"`
	Role       string                 `json:"role,omitempty"`
	ID         string                 `json:"id,omitempty"`
	SessionID  string                 `json:"session_id,omitempty"`
	Token      string                 `json:"token,omitempty"`
	ExpiresAt  string                 `json:"expires_at,omitempty"`
	Target     string                 `json:"target,omitempty"`
	HardwareID string                 `json:"hardware_id,omitempty"`
	Password   string                 `json:"password,omitempty"`
	Error      string                 `json:"error,omitempty"`
	ApiKey     string                 `json:"api_key,omitempty"`
}
