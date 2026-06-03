package main

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

var errUserAlreadyExists = errors.New("user already exists")

func (s *store) GetUserByUsername(username string) (*userRecord, error) {
	var user userRecord
	if err := s.db.Where("username = ?", username).Take(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *store) GetUserByID(id uint64) (*userRecord, error) {
	var user userRecord
	if err := s.db.Where("id = ?", id).Take(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *store) ListUsers() ([]userRecord, error) {
	var users []userRecord
	return users, s.db.Order("id ASC").Find(&users).Error
}

func (s *store) CreateAdminUser(username, password string) (*userRecord, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	if password == "" {
		return nil, fmt.Errorf("password is required")
	}
	if _, err := s.GetUserByUsername(username); err == nil {
		return nil, errUserAlreadyExists
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("hash user password: %w", err)
	}
	user := userRecord{Username: username, PasswordHash: hash, Role: adminRole}
	if err := s.db.Create(&user).Error; err != nil {
		return nil, err
	}
	return &user, nil
}

func (s *store) UpdateUserPassword(id uint64, password string) (*userRecord, error) {
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
	if err := s.db.Model(&userRecord{}).Where("id = ?", id).Updates(map[string]any{"password_hash": hash, "updated_at": time.Now()}).Error; err != nil {
		return nil, err
	}
	user.PasswordHash = hash
	return s.GetUserByID(id)
}

func (s *store) EnsureDefaultAdmin(username, password string) error {
	var existing userRecord
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
	user := userRecord{Username: username, PasswordHash: hash, Role: adminRole}
	if err := s.db.Create(&user).Error; err != nil {
		return fmt.Errorf("create default admin user: %w", err)
	}
	return nil
}
