package minienv

type InMemoryUserStore struct {
	UsersByAccessToken map[string]*User
}

func NewInMemoryUserStore() (*InMemoryUserStore) {
	return &InMemoryUserStore{
		UsersByAccessToken: make(map[string]*User),
	}
}

func (store InMemoryUserStore) SetUser(accessToken string, user *User) (error) {
	store.UsersByAccessToken[accessToken] = user
	return nil
}

func (store InMemoryUserStore) GetUser(accessToken string) (*User, error) {
	return store.UsersByAccessToken[accessToken], nil
}