package model

import "time"

type CasEntry struct {
	ID               string    `json:"id"`
	Hash             string    `json:"hash"`
	EncryptedContent string    `json:"-"`
	ContentType      string    `json:"contentType"`
	CreatedAt        time.Time `json:"createdAt"`
}
