package main

import (
	"app/assignment/controllers"
	"app/assignment/models"
	"encoding/csv"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
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

	// Connect to DB
	dbConn := dbUser + ":" + dbPassword + "@tcp" + "(" + dbHost + ":" + dbPort + ")/" + "?" + "parseTime=true&loc=Local"
	db, dbErr = gorm.Open(mysql.Open(dbConn), &gorm.Config{})
	if dbErr != nil {
		log.Fatal("Failed to connect to database: ", dbErr)
	} else {
		log.Println("Succesfully connected to MYSQL")
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal("Failed to get SQL database:", err)
	}
	defer sqlDB.Close()

	_, err = sqlDB.Exec("CREATE DATABASE IF NOT EXISTS " + dbName)
	if err != nil {
		log.Fatal("Failed to create the database:", err)
	} else {
		log.Println("Successfully cretaed db with name:", dbName)
	}

	dbConn = dbUser + ":" + dbPassword + "@tcp(" + dbHost + ":" + dbPort + ")/" + dbName + "?parseTime=true&loc=Local"
	db, err = gorm.Open(mysql.Open(dbConn), &gorm.Config{})
	if err != nil {
		log.Fatal("Failed to connect to the custom-named database:", err)
	} else {
		println("Successfully connected")
	}

	// Bootstrap db with schemas
	db.AutoMigrate(&models.Account{}, &models.Assignment{})

	file, err := os.Open(usersFilePath)
	if err != nil {
		println("FILE OPEN ERR")
		log.Fatal(err)
	}
	defer file.Close()
	reader := csv.NewReader(file)
	// Read and discard the header line
	_, err = reader.Read()
	if err != nil {
		log.Fatal(err)
	}

	for {
		record, err := reader.Read()
		if err != nil {
			break // Reached end of file
		}

		// Check if the email already exists in the database
		var existingAccount models.Account
		if db.First(&existingAccount, "email = ?", record[2]).Error == nil {
			// User with the same email already exists, skip this record
			log.Printf("User with email %s already exists, skipping", record[2])
			continue
		}

		// Hash the password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(record[3]), bcrypt.DefaultCost)
		if err != nil {
			log.Printf("Error hashing password: %v", err)
			continue
		}

		acc1 := models.Account{
			Firstname: record[0],
			LastName:  record[1],
			Email:     record[2],
			Password:  string(hashedPassword),
		}
		db.Create(&acc1)
	}

	// router setup
	router := gin.Default()

	router.Any("/healthz", healthCheck)

	router.POST("/v1/assignments", createAssignment)

	router.GET("/v1/assignments", getAllAssignments)

	router.GET("/v1/assignments/:id", getAssignment)

	router.PUT("/v1/assignments/:id", updateAssignment)

	router.PATCH("/v1/assignments/:id", func(c *gin.Context) {
		c.JSON(http.StatusMethodNotAllowed, gin.H{"error": "PATCH not allowed!"})
	})

	router.DELETE("/v1/assignments/:id", deleteAssignment)

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

func createAssignment(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	var assignmentInput models.AssignmentInput

	// Bind the request body to the `assignmentInput` struct
	if err := c.BindJSON(&assignmentInput); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Authenticate the user and obtain their user ID
	//userID, err := authenticateUser(c)
	userID, err := controllers.AuthenticateUser(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication Failed!"})
		return
	}

	//Max Point CriteriaCheck
	if assignmentInput.Points <=
		0 || assignmentInput.Points > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Assignment Points should be between 1 and 100"})
		return
	}

	// NoOfPOints CriteriaCheck
	if assignmentInput.NoOfAttempts <=
		0 || assignmentInput.NoOfAttempts > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No of attempts should be between 1 and 100"})
		return
	}

	// Set the UserID field in the Assignment struct
	newAssignment := models.Assignment{
		Name:         assignmentInput.Name,
		Points:       assignmentInput.Points,
		NoOfAttempts: assignmentInput.NoOfAttempts,
		Deadline:     assignmentInput.Deadline,
		AccountID:    userID,
	}

	// Create a new assignment record in the database
	result := db.Create(&newAssignment)
	if result.Error != nil {
		c.JSON(http.StatusExpectationFailed, gin.H{"error": "An error occured while creating a new assignment"})
	}

	c.JSON(http.StatusCreated, assignmentInput)

}

func getAllAssignments(c *gin.Context) {

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	// Authenticate the user and obtain their user ID

	_, err := controllers.AuthenticateUser(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication Failed!"})
		return
	}

	// Query the database to retrieve all assignments
	var assignments []models.Assignment
	if err := db.Find(&assignments).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var assignmentResponses []models.AssignmentResponse

	for _, ass := range assignments {
		assResp := models.AssignmentResponse{
			ID:                ass.ID,
			Name:              ass.Name,
			Deadline:          ass.Deadline,
			Points:            ass.Points,
			NoOfAttempts:      ass.NoOfAttempts,
			AssignemtCreated:  ass.CreatedAt.String(),
			AssignmentUpdated: ass.UpdatedAt.String(),
		}
		assignmentResponses = append(assignmentResponses, assResp)
	}

	// Return the list of assignments as a JSON response
	c.JSON(http.StatusOK, assignmentResponses)
}

func getAssignment(c *gin.Context) {

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	_, err := controllers.AuthenticateUser(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication Failed!"})
		return
	}

	assID := c.Param("id")

	// Parse the assignment ID as an integer
	id, err := strconv.ParseUint(assID, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assignment ID"})
		return
	}

	// Query the database to find the assignment by ID
	var assignment models.Assignment
	if err := db.First(&assignment, id).Error; err != nil {
		if gorm.ErrRecordNotFound == err {
			c.JSON(http.StatusNotFound, gin.H{"error": "Assignment not found"})
			return
		}
	}

	assResp := models.AssignmentResponse{
		ID:                assignment.ID,
		Name:              assignment.Name,
		Deadline:          assignment.Deadline,
		Points:            assignment.Points,
		NoOfAttempts:      assignment.NoOfAttempts,
		AssignemtCreated:  assignment.CreatedAt.String(),
		AssignmentUpdated: assignment.UpdatedAt.String(),
	}

	// Return the assignment as a JSON response
	c.JSON(http.StatusOK, assResp)
}

func deleteAssignment(c *gin.Context) {

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	// Authenticate the user and obtain their user ID
	userID, err := controllers.AuthenticateUser(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Extract the assignment ID from the URL parameter
	assignmentID := c.Param("id")
	// Parse the assignment ID as an integer
	id, err := strconv.Atoi(assignmentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assignment ID"})
		return
	}

	var assignment models.Assignment
	if err := db.First(&assignment, id).Error; err != nil {
		if gorm.ErrRecordNotFound == err {
			c.JSON(http.StatusNotFound, gin.H{"error": "Assignment not found"})
			return
		}
	}

	// Check if the authenticated user is the owner of the assignment
	if assignment.AccountID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not authorized to delete this assignment"})
		return
	}

	if err := db.Delete(&assignment).Error; err != nil {
		c.JSON(http.StatusExpectationFailed, gin.H{"error": "Failed to delete the assignment"})
		return
	}

	c.JSON(http.StatusNoContent, gin.H{"message": "Assignment deleted successfully"})

}

func updateAssignment(c *gin.Context) {

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	// Authenticate the user and obtain their user ID
	userID, err := controllers.AuthenticateUser(c, db)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization Failed"})
		return
	}

	// Get the assignment ID from the request parameters
	assignmentIDStr := c.Param("id")
	assignmentID, err := strconv.ParseUint(assignmentIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assignment ID"})
		return
	}

	// Check if the assignment exists and retrieve its owner's UserID
	var assignment models.Assignment
	if err := db.Where("id = ?", assignmentID).First(&assignment).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Assignment not found"})
		return
	}

	// Check if the authenticated user is the owner of the assignment
	if assignment.AccountID != userID {
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not authorized to update this assignment"})
		return
	}

	// Bind the request body to the `AssignmentInput` struct
	var input models.AssignmentInput
	if err := c.BindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Update the assignment fields with the input data
	assignment.Name = input.Name
	assignment.Points = input.Points
	assignment.NoOfAttempts = input.NoOfAttempts
	assignment.Deadline = input.Deadline

	// Save the updated assignment to the database
	if err := db.Save(&assignment).Error; err != nil {
		c.JSON(http.StatusExpectationFailed, gin.H{"error": "Failed to update the assignment"})
		return
	}

	assResp := models.AssignmentResponse{
		ID:                assignment.ID,
		Name:              assignment.Name,
		Points:            assignment.Points,
		Deadline:          assignment.Deadline,
		NoOfAttempts:      assignment.NoOfAttempts,
		AssignemtCreated:  assignment.CreatedAt.String(),
		AssignmentUpdated: assignment.UpdatedAt.String(),
	}

	c.JSON(http.StatusOK, assResp)
}
