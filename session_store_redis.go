package minienv

import (
	"log"
	"github.com/go-redis/redis"
	"encoding/json"
	"strconv"
)

type RedisSessionStore struct {
	Client *redis.Client
}

func NewRedisSessionStore(address string, password string, dbStr string) (*RedisSessionStore, error) {
	db, _ := strconv.ParseInt(dbStr, 10, 64)
	client := redis.NewClient(&redis.Options{
		Addr: address,
		Password: password,
		DB: int(db),
	})
	_, err := client.Ping().Result()
	if err != nil {
		log.Printf("Failed to ping Redis: %v\n", err)
		return nil, err
	}
	return &RedisSessionStore{
		Client: client,
	}, nil
}

func (store RedisSessionStore) SetSession(id string, session *Session) (error) {
	bs, err := json.Marshal(session)
	if err != nil {
		return err
	}
	log.Printf("Redis setting session: %s\n", string(bs))
	err = store.Client.Set(id, bs, 0).Err()
	if err != nil {
		log.Printf("Redis error setting session: %v\n", err)
		return err
	}
	return nil
}

func (store RedisSessionStore) GetSession(id string) (*Session, error) {
	bs, err := store.Client.Get(id).Bytes()
	if err != nil {
		return nil, err
	}
	log.Printf("Redis getting session: %s\n", string(bs))
	var session Session
	err = json.Unmarshal(bs, &session)
	if err != nil {
		log.Printf("Redis error getting session: %v\n", err)
		return nil, err
	}
	return &session, nil
}