package entities

import "time"

// PricingChannel is Keeper-owned; provider labels and endpoints are not IDs.
type PricingChannel struct {
	ID        string    `gorm:"primaryKey"`
	Name      string    `gorm:"not null"`
	CreatedAt time.Time `gorm:"serializer:storageTime"`
	UpdatedAt time.Time `gorm:"serializer:storageTime"`
}

// A registered credential subject can belong to at most one channel.
type PricingChannelMember struct {
	SubjectID string `gorm:"primaryKey"`
	ChannelID string `gorm:"not null;index"`
}

// No row means inheritance, distinct from explicit zero and one.
type ChannelPriceDefault struct {
	ChannelID  string    `gorm:"primaryKey"`
	Multiplier float64   `gorm:"not null"`
	UpdatedAt  time.Time `gorm:"serializer:storageTime"`
}
