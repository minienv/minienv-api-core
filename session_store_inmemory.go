package minienv

type InMemorySessionStore struct {
	SessionsById map[string]*Session
}

func NewInMemorySessionStore() (*InMemorySessionStore) {
	return &InMemorySessionStore{
		SessionsById: make(map[string]*Session),
	}
}

func (store InMemorySessionStore) SetSession(id string, session *Session) (error) {
	store.SessionsById[id] = session
	return nil
}

func (store InMemorySessionStore) GetSession(id string) (*Session, error) {
	return store.SessionsById[id], nil
}