package minienv

type SessionStore interface {
	SetSession(id string, session *Session) (error)
	GetSession(id string) (*Session, error)
}

type Session struct {
	Id string  `json:"sessionId"`
	EnvId string `json:"envId"`
	EnvServiceName string `json:"envServiceName"`
	Props map[string]interface{}
}