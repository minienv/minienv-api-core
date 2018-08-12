package minienv

import "net/http"

type AuthHandlerFunc func(http.ResponseWriter, *http.Request, *User, *Session)

type AuthProvider interface {
	OnAuthCallback(parameters map[string][]string) (*User, error)
	LoginUser(accessToken string) (*User, error)
	UserCanViewRepo(user *User, repo string) (bool, error)
}

type SessionStore interface {
	SetSession(id string, session *Session) (error)
	GetSession(id string) (*Session, error)
}

type UserStore interface {
	SetUser(accessToken string, user *User) (error)
	GetUser(accessToken string) (*User, error)
}

type User struct {
	AccessToken string `json:"accessToken"`
	Email string `json:"email"`
	Username string `json:"username"`
}

type Session struct {
	Id string  `json:"sessionId"`
	User *User `json:"user"`
	EnvId string `json:"envId"`
	EnvServiceName string `json:"envServiceName"`
}
