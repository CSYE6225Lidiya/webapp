package models

import "gorm.io/gorm"

type Account struct {
	gorm.Model
	Firstname   string       `gorm:"size:225;not null" json:"firstname"`
	LastName    string       `gorm:"size:225;not null" json:"lastname"`
	Email       string       `gorm:"size:225;not null;unique" json:"email"`
	Password    string       `gorm:"size:225;not null" json:"password"`
	Assignments []Assignment // one to maany relationship
}

type Assignment struct {
	gorm.Model
	Name         string  `json:"name"`
	Points       int     `json:"points"`
	NoOfAttempts int     `json:"noofattempts"`
	Deadline     string  `json:"deadline"`
	AccountID    uint    // Foreign key to Account table
	Account      Account `gorm:"foreignKey:AccountID"`
}

type AssignmentInput struct {
	Name         string `json:"name"`
	Points       int    `json:"points"`
	NoOfAttempts int    `json:"noofattempts"`
	Deadline     string `json:"deadline"`
	//AccountID    uint   // Foreign key to Account table
}

type AssignmentResponse struct {
	ID                uint
	Name              string `json:"name"`
	Points            int    `json:"points"`
	NoOfAttempts      int    `json:"noofattempts"`
	Deadline          string `json:"deadline"`
	AssignemtCreated  string
	AssignmentUpdated string
}

type Submission struct {
	gorm.Model
	AssignmentID      uint64     // Foreign Key to Assignment Table
	Assignment        Assignment `gorm:"foreignKey:AssignmentID"`
	AccountID         uint       // Foreign key to Account table
	Account           Account    `gorm:"foreignKey:AccountID"`
	SubmissionUrl     string     `json:"submission_url"`
	SubmissionRetries int
}

type SubmissionInput struct {
	SubmissionUrl string `json:"submission_url"`
}

type SubmissionResponse struct {
	ID                uint
	AssignmentID      uint   `json:"assignment_id"`
	SubmissionUrl     string `json:"submission_url"`
	SubmissionDate    string `json:"submission_date"`
	SubmissionRetries int    `json:"submission_retries"`
}
