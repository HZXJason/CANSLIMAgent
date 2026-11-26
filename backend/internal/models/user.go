package models

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const UserStatusActive = "active"

var DefaultUserID = uuid.MustParse("00000000-0000-4000-8000-000000000001")

type User struct {
	ID           uuid.UUID `json:"id" gorm:"type:uuid;primaryKey"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	DisplayName  string    `json:"display_name" gorm:"type:text;not null"`
	Timezone     string    `json:"timezone" gorm:"type:text;not null"`
	BaseCurrency string    `json:"base_currency" gorm:"type:text;not null"`
	Status       string    `json:"status" gorm:"type:text;not null;index"`
}

func DefaultUser() User {
	return User{
		ID: DefaultUserID, DisplayName: "AlphaLens User", Timezone: "Asia/Shanghai",
		BaseCurrency: "USD", Status: UserStatusActive,
	}
}

func (user *User) BeforeCreate(_ *gorm.DB) error {
	if user.ID == uuid.Nil {
		user.ID = uuid.New()
	}
	return nil
}

func ensureDefaultUserID(userID *uuid.UUID) {
	if *userID == uuid.Nil {
		*userID = DefaultUserID
	}
}
