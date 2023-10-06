package main

import (
	"fmt"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var dbUser = os.Getenv("DB_USER")
var dbPassword = os.Getenv("DB_PASSWORD")
var dbName = os.Getenv("DB_NAME")
var dbHost = os.Getenv("DB_HOST")
var dbPort = os.Getenv("DB_PORT")
var usersFilePath = os.Getenv("USERS_PATH")

var db *gorm.DB
var dbErr error

func main() {

	// router setup
	router := gin.Default()

	router.Any("/healthz", healthCheck)

	router.Run()

}

func healthCheck(c *gin.Context) {

	// Set Cache Control
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	// Check for http method
	if c.Request.Method != http.MethodGet {
		c.Status((http.StatusMethodNotAllowed))
		return
	}

	// Payload Check
	if c.Request.ContentLength > 0 {
		c.Status(http.StatusBadRequest)
		return
	}

	// Connection String
	dbConn := dbUser + ":" + dbPassword + "@tcp" + "(" + dbHost + ":" + dbPort + ")/" + dbName + "?" + "parseTime=true&loc=Local"
	fmt.Println("DB Connection String ", dbConn)
	_, dbConnErr := gorm.Open(mysql.Open(dbConn), &gorm.Config{})
	fmt.Printf("DB Connection Status: error=%v", dbConnErr)

	// DB Connection Check
	if dbConnErr != nil {
		c.Status(http.StatusServiceUnavailable)
	} else {
		c.Status(http.StatusOK)
	}

}
