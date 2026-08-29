package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/R1ddle1337/sb-manager-web/internal/types"
	"go.etcd.io/bbolt"
)

var (
	bucketUsers       = []byte("users")
	bucketSessions    = []byte("sessions")
	bucketServers     = []byte("servers")
	bucketTasks       = []byte("tasks")
	bucketEnrollments = []byte("enrollments")
)

type Store struct{ db *bbolt.DB }

func Open(path string) (*Store, error) {
	db, err := bbolt.Open(path, 0600, &bbolt.Options{Timeout: 3 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	s := &Store{db: db}
	if err := db.Update(func(tx *bbolt.Tx) error {
		for _, bucket := range [][]byte{bucketUsers, bucketSessions, bucketServers, bucketTasks, bucketEnrollments} {
			if _, err := tx.CreateBucketIfNotExists(bucket); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

func putJSON(tx *bbolt.Tx, bucket, key []byte, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return tx.Bucket(bucket).Put(key, data)
}

func getJSON(tx *bbolt.Tx, bucket, key []byte, value any) error {
	data := tx.Bucket(bucket).Get(key)
	if data == nil {
		return bbolt.ErrBucketNotFound
	}
	return json.Unmarshal(append([]byte(nil), data...), value)
}

func (s *Store) PutUser(user types.User) error {
	return s.db.Update(func(tx *bbolt.Tx) error { return putJSON(tx, bucketUsers, []byte(user.Username), user) })
}

func (s *Store) GetUser(username string) (types.User, error) {
	var user types.User
	err := s.db.View(func(tx *bbolt.Tx) error { return getJSON(tx, bucketUsers, []byte(username), &user) })
	return user, err
}

func (s *Store) HasUsers() (bool, error) {
	var found bool
	err := s.db.View(func(tx *bbolt.Tx) error { found = tx.Bucket(bucketUsers).Stats().KeyN > 0; return nil })
	return found, err
}

func (s *Store) PutSession(session types.Session) error {
	return s.db.Update(func(tx *bbolt.Tx) error { return putJSON(tx, bucketSessions, []byte(session.ID), session) })
}

func (s *Store) GetSession(id string) (types.Session, error) {
	var session types.Session
	err := s.db.View(func(tx *bbolt.Tx) error { return getJSON(tx, bucketSessions, []byte(id), &session) })
	return session, err
}

func (s *Store) DeleteSession(id string) error {
	return s.db.Update(func(tx *bbolt.Tx) error { return tx.Bucket(bucketSessions).Delete([]byte(id)) })
}

func (s *Store) PutServer(server types.Server) error {
	return s.db.Update(func(tx *bbolt.Tx) error { return putJSON(tx, bucketServers, []byte(server.ID), server) })
}

func (s *Store) GetServer(id string) (types.Server, error) {
	var server types.Server
	err := s.db.View(func(tx *bbolt.Tx) error { return getJSON(tx, bucketServers, []byte(id), &server) })
	return server, err
}

func (s *Store) ListServers() ([]types.Server, error) {
	servers := []types.Server{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketServers).ForEach(func(_, data []byte) error {
			var server types.Server
			if err := json.Unmarshal(data, &server); err != nil {
				return err
			}
			server.AgentPublicKey = ""
			servers = append(servers, server)
			return nil
		})
	})
	return servers, err
}

func (s *Store) DeleteServer(id string) error {
	return s.db.Update(func(tx *bbolt.Tx) error { return tx.Bucket(bucketServers).Delete([]byte(id)) })
}

func (s *Store) PutTask(task types.Task) error {
	return s.db.Update(func(tx *bbolt.Tx) error { return putJSON(tx, bucketTasks, []byte(task.ID), task) })
}

func (s *Store) UpdateTask(id string, update func(*types.Task) error) (types.Task, error) {
	var result types.Task
	err := s.db.Update(func(tx *bbolt.Tx) error {
		if err := getJSON(tx, bucketTasks, []byte(id), &result); err != nil {
			return err
		}
		if err := update(&result); err != nil {
			return err
		}
		return putJSON(tx, bucketTasks, []byte(id), result)
	})
	return result, err
}

func (s *Store) ClaimPendingTask(serverID string) (types.Task, error) {
	var result types.Task
	err := s.db.Update(func(tx *bbolt.Tx) error {
		bucket := tx.Bucket(bucketTasks)
		return bucket.ForEach(func(key, data []byte) error {
			if result.ID != "" {
				return nil
			}
			var task types.Task
			if err := json.Unmarshal(data, &task); err != nil {
				return err
			}
			if task.ServerID != serverID || task.Status != types.TaskPending {
				return nil
			}
			now := time.Now().UTC()
			task.Status = types.TaskRunning
			task.StartedAt = &now
			if err := putJSON(tx, bucketTasks, key, task); err != nil {
				return err
			}
			result = task
			return nil
		})
	})
	if err != nil {
		return types.Task{}, err
	}
	if result.ID == "" {
		return types.Task{}, bbolt.ErrBucketNotFound
	}
	return result, nil
}

func (s *Store) GetTask(id string) (types.Task, error) {
	var task types.Task
	err := s.db.View(func(tx *bbolt.Tx) error { return getJSON(tx, bucketTasks, []byte(id), &task) })
	return task, err
}

func (s *Store) ListTasks(limit int) ([]types.Task, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	tasks := []types.Task{}
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketTasks).ForEach(func(_, data []byte) error {
			if len(tasks) >= limit {
				return nil
			}
			var task types.Task
			if err := json.Unmarshal(data, &task); err != nil {
				return err
			}
			tasks = append(tasks, task)
			return nil
		})
	})
	return tasks, err
}

func (s *Store) FindTaskByIdempotency(key string) (types.Task, error) {
	if key == "" {
		return types.Task{}, bbolt.ErrBucketNotFound
	}
	var found types.Task
	err := s.db.View(func(tx *bbolt.Tx) error {
		return tx.Bucket(bucketTasks).ForEach(func(_, data []byte) error {
			var task types.Task
			if err := json.Unmarshal(data, &task); err != nil {
				return err
			}
			if task.IdempotencyKey == key {
				found = task
			}
			return nil
		})
	})
	if err != nil {
		return types.Task{}, err
	}
	if found.ID == "" {
		return types.Task{}, bbolt.ErrBucketNotFound
	}
	return found, nil
}

func (s *Store) PutEnrollment(key string, token types.EnrollmentToken) error {
	return s.db.Update(func(tx *bbolt.Tx) error { return putJSON(tx, bucketEnrollments, []byte(key), token) })
}

func (s *Store) GetEnrollment(key string) (types.EnrollmentToken, error) {
	var token types.EnrollmentToken
	err := s.db.View(func(tx *bbolt.Tx) error { return getJSON(tx, bucketEnrollments, []byte(key), &token) })
	return token, err
}

func (s *Store) MarkEnrollmentUsed(key string) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		var token types.EnrollmentToken
		if err := getJSON(tx, bucketEnrollments, []byte(key), &token); err != nil {
			return err
		}
		token.Used = true
		return putJSON(tx, bucketEnrollments, []byte(key), token)
	})
}

func IsNotFound(err error) bool { return errors.Is(err, bbolt.ErrBucketNotFound) }
