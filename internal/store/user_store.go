package store

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrUserAlreadyExists = errors.New("user already exists")

func (s *Store) GetUserByUsername(username string) (*UserRecord, error) {
	var user UserRecord
	if err := s.db.Where("username = ?", username).Take(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) GetUserByID(id uint64) (*UserRecord, error) {
	var user UserRecord
	if err := s.db.Where("id = ?", id).Take(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) ListUsers() ([]UserRecord, error) {
	var users []UserRecord
	return users, s.db.Order("id ASC").Find(&users).Error
}

func (s *Store) CreateAdminUser(username, password string) (*UserRecord, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if password == "" {
		return nil, fmt.Errorf("password is required")
	}
	if _, err := s.GetUserByUsername(username); err == nil {
		return nil, ErrUserAlreadyExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash user password: %w", err)
	}
	user := UserRecord{Username: username, PasswordHash: hash, Role: AdminRole}
	if err := s.db.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *Store) UpdateUserPassword(id uint64, password string) (*UserRecord, error) {
	if id == 0 {
		return nil, fmt.Errorf("user id is required")
	}
	if password == "" {
		return nil, fmt.Errorf("password is required")
	}
	user, err := s.GetUserByID(id)
	if err != nil {
		return nil, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash user password: %w", err)
	}
	if err := s.db.Model(&UserRecord{}).Where("id = ?", id).Updates(map[string]any{"password_hash": hash, "updated_at": time.Now()}).Error; err != nil {
		return nil, err
	}
	user.PasswordHash = hash
	return s.GetUserByID(id)
}

func (s *Store) EnsureDefaultAdmin(username, password string) error {
	var existing UserRecord
	err := s.db.Where("username = ?", username).Take(&existing).Error
	if err == nil {
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return fmt.Errorf("hash admin password: %w", err)
	}
	user := UserRecord{Username: username, PasswordHash: hash, Role: AdminRole}
	if err := s.db.Create(&user).Error; err != nil {
		return fmt.Errorf("create default admin user: %w", err)
	}
	return nil
}
