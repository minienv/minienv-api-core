package minienv

import "net/http"

type AuthHandlerFunc func(http.ResponseWriter, *http.Request, *User, *Session)

type AuthProvider interface {
	onAuthCallback(parameters map[string][]string) (*User, error)
	loginUser(accessToken string) (*User, error)
	userCanViewRepo(user *User, repo string) (bool, error)
}

type SessionStore interface {
	setSession(id string, session *Session) (error)
	getSession(id string) (*Session, error)
}

type UserStore interface {
	setUser(accessToken string, user *User) (error)
	getUser(accessToken string) (*User, error)
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
