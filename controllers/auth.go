package controllers

import (
	"app/assignment/models"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

func AuthenticateUser(c *gin.Context, db *gorm.DB) (uint, error) {
	user, password, _ := c.Request.BasicAuth()

	// Query the database for the user
	var currentUser models.Account
	if err := db.Where("email = ?", user).First(&currentUser).Error; err != nil {
		println("ERR CURR USER")
		c.JSON(http.StatusUnauthorized, gin.H{"message": "User not found"})
		c.Abort()
		return 0, fmt.Errorf("USER NOT FOUND")
	}

	// Compare the provided password with the stored bcrypt hash
	if err := bcrypt.CompareHashAndPassword([]byte(currentUser.Password), []byte(password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "Invalid credentials"})
		c.Abort()
		return 0, fmt.Errorf("INVALID CREDENTIALS")
	}

	return currentUser.ID, nil
}
