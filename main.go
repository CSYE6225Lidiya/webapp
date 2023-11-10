package main

import (
	"app/assignment/controllers"
	"app/assignment/models"
	"encoding/csv"
	"errors"
	"fmt"
	"io/ioutil"

	"net/http"
	"os"
	"strconv"

	statsd "github.com/etsy/statsd/examples/go"
	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var dbUser string = os.Getenv("DB_USER")
var dbPassword string = os.Getenv("DB_PASSWORD")
var dbName string = os.Getenv("DB_NAME")
var dbHost string = os.Getenv("DB_HOST")
var dbPort string = os.Getenv("DB_PORT")

var db *gorm.DB
var dbErr error

type DbConfig struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	DB       string `yaml:"db"`
}

// Initialize the StatsD client
var statsdClient = statsd.New("127.0.0.1", 8125)

func main() {

	// Open the log file for writing
	logFile, err := os.OpenFile("app.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
	if err != nil {
		fmt.Printf("Failed to open log file: %v\n", err)
		log.Error().Err(err).Msg("Unable to open log file for writing logs")
		return
	}
	defer logFile.Close()

	// Set zerolog to write logs to the file
	log.Logger = log.Output(logFile)

	log.Info().Msg("Successfully created the log file for the webapp")

	yamlFile, err := ioutil.ReadFile("/opt/dbconfig.yaml")
	var dbconfig DbConfig
	if err == nil {
		println("Got File--Unmarshalling it")
		if err := yaml.Unmarshal(yamlFile, &dbconfig); err != nil {
			fmt.Printf("Unmrshal Unsuccessful: error=%v", err)
			log.Error().Err(err).Msg("Unable to UNMARSHAL the given config file")
		} else {
			println("Setting file values into conn details from config file")
			log.Info().Msg("Successfully read the database configuration given and setting up the values for connection")
			dbUser = dbconfig.User
			dbPassword = dbconfig.Password
			dbName = dbconfig.DB
			dbHost = dbconfig.Host
			dbPort = dbconfig.Port
		}
	}

	// Connect to DB
	dbConn := dbUser + ":" + dbPassword + "@tcp" + "(" + dbHost + ":" + dbPort + ")/" + "?" + "parseTime=true&loc=Local"
	db, dbErr = gorm.Open(mysql.Open(dbConn), &gorm.Config{})
	if dbErr != nil {
		log.Error().Err(err).Msg("Unable to connect to database with given connection data")
	} else {
		log.Info().Msg("Successfully connected to database")
	}

	sqlDB, err := db.DB()
	if err != nil {
		log.Error().Err(err).Msg("Unable to create a sql database object")
	}
	defer sqlDB.Close()

	_, err = sqlDB.Exec("CREATE DATABASE IF NOT EXISTS " + dbName)
	if err != nil {
		log.Error().Err(err).Str("database", dbName).Msg("Failed to create the database")
	} else {
		log.Info().Str("database", dbName).Msg("Successfully created database")
	}

	dbConn = dbUser + ":" + dbPassword + "@tcp(" + dbHost + ":" + dbPort + ")/" + dbName + "?parseTime=true&loc=Local"
	db, err = gorm.Open(mysql.Open(dbConn), &gorm.Config{})
	if err != nil {

		log.Error().Err(err).Str("database", dbName).Msg("Failed to connect to the custom-named database")
	} else {
		log.Info().Str("database", dbName).Msg("Successfully connected to database")
	}

	// Bootstrap db with schemas
	db.AutoMigrate(&models.Account{}, &models.Assignment{})

	file, err := os.Open("users.csv")
	if err != nil {
		println("FILE OPEN ERR")
		log.Error().Err(err).Str("file", "users.csv").Msg("Failed to open the users file given")
	}
	defer file.Close()
	reader := csv.NewReader(file)
	// Read and discard the header line
	_, err = reader.Read()
	if err != nil {
		log.Error().Err(err).Str("file", "users.csv").Msg("Unable to discard the header line from users file")
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
			//log.Printf("User with email %s already exists, skipping", record[2])
			log.Info().Str("email", record[2]).Msg("User with email already exists, skipping")
			continue
		}

		// Hash the password
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(record[3]), bcrypt.DefaultCost)
		if err != nil {
			log.Error().Err(err).Msg("Error in hashing the password")
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

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("Healthz Endpoint")

	// Check for http method
	if c.Request.Method != http.MethodGet {
		c.Status((http.StatusMethodNotAllowed))
		err := errors.New("METHOD NOT ALLOWD")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("Healthz Endpoint:Wrong Method")
		return
	}

	// Payload Check
	if c.Request.ContentLength > 0 {
		c.Status(http.StatusBadRequest)
		err := errors.New("PAYLOAD NOT ALLOWED")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("Healthz Endpoint:Content length greater than zero")
		return
	}

	// Connection String
	dbConn := dbUser + ":" + dbPassword + "@tcp" + "(" + dbHost + ":" + dbPort + ")/" + dbName + "?" + "parseTime=true&loc=Local"
	fmt.Println("DB Connection String ", dbConn)
	_, dbConnErr := gorm.Open(mysql.Open(dbConn), &gorm.Config{})
	fmt.Printf("DB Connection Status: error=%v", dbConnErr)

	// DB Connection Check
	if dbConnErr != nil {
		err := errors.New("DATABASE CONNECTION ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("Healthz Endpoint:Unable to connect to database")
		c.Status(http.StatusServiceUnavailable)
	} else {
		log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("Healthz Endpoint:Successfully connected to database")
		c.Status(http.StatusOK)
	}

}

func createAssignment(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("CreateAssignment Endpoint")

	var assignmentInput models.AssignmentInput

	// Bind the request body to the `assignmentInput` struct
	if err := c.BindJSON(&assignmentInput); err != nil {
		err := errors.New("INCORRECT REQUEST BODY")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("CreateAssignment Endpoint:The request body is incorrect")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	// Authenticate the user and obtain their user ID
	//userID, err := authenticateUser(c)
	userID, err := controllers.AuthenticateUser(c, db)
	if err != nil {
		err := errors.New("AUTHENTICATION ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("CreateAssignment Endpoint:Unable to authenticate the request")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication Failed!"})
		return
	}

	//Max Point CriteriaCheck
	if assignmentInput.Points <=
		0 || assignmentInput.Points > 100 {
		err := errors.New("ASSIGNMENT POINTS ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("CreateAssignment Endpoint:Assignment Points should be between 1 and 100")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Assignment Points should be between 1 and 100"})
		return
	}

	// NoOfPOints CriteriaCheck
	if assignmentInput.NoOfAttempts <=
		0 || assignmentInput.NoOfAttempts > 100 {
		err := errors.New("NUMBER OF ATTEMPTS ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("CreateAssignment Endpoint:No of attempts should be between 1 and 100")
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
		err := errors.New("ASSIGNMENT CREATION ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("CreateAssignment Endpoint:An error occured while creating a new assignment")
		c.JSON(http.StatusExpectationFailed, gin.H{"error": "An error occured while creating a new assignment"})
	}

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("CreateAssignment Endpoint:Successfully created the assignment")

	c.JSON(http.StatusCreated, assignmentInput)

}

func getAllAssignments(c *gin.Context) {

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("GetAllAssignments Endpoint")

	// Authenticate the user and obtain their user ID

	_, err := controllers.AuthenticateUser(c, db)
	if err != nil {
		err := errors.New("AUTHENTICATION ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("GetAllAssignments Endpoint:Unable to authenticate the request")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication Failed!"})
		return
	}

	// Query the database to retrieve all assignments
	var assignments []models.Assignment
	if err := db.Find(&assignments).Error; err != nil {
		err := errors.New("ASSIGNMENT RETRIEVAL ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("GetAllAssignments Endpoint:Unable to retrieve errrors from database")
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

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("GetAllAssignments Endpoint:Successfully retrieved all assignments")

	// Return the list of assignments as a JSON response
	c.JSON(http.StatusOK, assignmentResponses)
}

func getAssignment(c *gin.Context) {

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("GetAnAssignment Endpoint")

	_, err := controllers.AuthenticateUser(c, db)
	if err != nil {
		err := errors.New("AUTHENTICATION ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("GetAnAssignment Endpoint:Unable to authenticate the request")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authentication Failed!"})
		return
	}

	assID := c.Param("id")

	// Parse the assignment ID as an integer
	id, err := strconv.ParseUint(assID, 10, 64)
	if err != nil {
		err := errors.New("INVALID ASSIGNMENT ID")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("GetAnAssignment Endpoint:The assignment ID is Invalid")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assignment ID"})
		return
	}

	// Query the database to find the assignment by ID
	var assignment models.Assignment
	if err := db.First(&assignment, id).Error; err != nil {
		if gorm.ErrRecordNotFound == err {
			err := errors.New("ASSIGNMENT NOT FOUND")
			log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("GetAnAssignment Endpoint:The assignment doesn't exist")
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

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("GetAnAssignment Endpoint:Successfullt retrieved the assignment")
	// Return the assignment as a JSON response
	c.JSON(http.StatusOK, assResp)
}

func deleteAssignment(c *gin.Context) {

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("DeleteAssignment Endpoint")

	// Authenticate the user and obtain their user ID
	userID, err := controllers.AuthenticateUser(c, db)
	if err != nil {
		err := errors.New("AUTHENTICATION ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("DeleteAssignment Endpoint:Unable to authenticate the request")
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}

	// Extract the assignment ID from the URL parameter
	assignmentID := c.Param("id")
	// Parse the assignment ID as an integer
	id, err := strconv.Atoi(assignmentID)
	if err != nil {
		err := errors.New("INVALID ASSIGNMENT ID")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("DeleteAssignment Endpoint:The assignment ID is Invalid")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assignment ID"})
		return
	}

	var assignment models.Assignment
	if err := db.First(&assignment, id).Error; err != nil {
		if gorm.ErrRecordNotFound == err {
			err := errors.New("ASSIGNMENT NOT FOUND")
			log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("DeleteAssignment Endpoint:The assignment doesn't exist")
			c.JSON(http.StatusNotFound, gin.H{"error": "Assignment not found"})
			return
		}
	}

	// Check if the authenticated user is the owner of the assignment
	if assignment.AccountID != userID {
		err := errors.New("AUTHORIZATION ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("DeleteAssignment Endpoint:The user is not authorized to delete this assignment")
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not authorized to delete this assignment"})
		return
	}

	if err := db.Delete(&assignment).Error; err != nil {
		err := errors.New("DELETE ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("DeleteAssignment Endpoint:Failed to delete the assignment")
		c.JSON(http.StatusExpectationFailed, gin.H{"error": "Failed to delete the assignment"})
		return
	}

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("DeleteAssignment Endpoint:Successfully deleted the assignment")

	c.JSON(http.StatusNoContent, gin.H{"message": "Assignment deleted successfully"})

}

func updateAssignment(c *gin.Context) {

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("UpdateAssignment Endpoint")

	// Authenticate the user and obtain their user ID
	userID, err := controllers.AuthenticateUser(c, db)
	if err != nil {
		err := errors.New("AUTHENTICATION ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("UpdateAssignment Endpoint:Unable to authenticate the request")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization Failed"})
		return
	}

	// Get the assignment ID from the request parameters
	assignmentIDStr := c.Param("id")
	assignmentID, err := strconv.ParseUint(assignmentIDStr, 10, 64)
	if err != nil {
		err := errors.New("INVALID ASSIGNMENT ID")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("UpdateAssignment Endpoint:The assignment ID is Invalid")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assignment ID"})
		return
	}

	// Check if the assignment exists and retrieve its owner's UserID
	var assignment models.Assignment
	if err := db.Where("id = ?", assignmentID).First(&assignment).Error; err != nil {
		err := errors.New("ASSIGNMENT NOT FOUND")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("UpdateAssignment Endpoint:The assignment doesn't exist")
		c.JSON(http.StatusNotFound, gin.H{"error": "Assignment not found"})
		return
	}

	// Check if the authenticated user is the owner of the assignment
	if assignment.AccountID != userID {
		err := errors.New("AUTHORIZATION ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("UpdateAssignment Endpoint:The user is not authorized to delete this assignment")
		c.JSON(http.StatusForbidden, gin.H{"error": "You are not authorized to update this assignment"})
		return
	}

	// Bind the request body to the `AssignmentInput` struct
	var input models.AssignmentInput
	if err := c.BindJSON(&input); err != nil {
		err := errors.New("INCORRECT REQUEST BODY")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("UpdateAssignment Endpoint:The request body is incorrect")
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
		err := errors.New("DELETE ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("UpdateAssignment Endpoint:Failed to update the assignment")
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

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("UpdateAssignment Endpoint:Successfully updated the assignment")

	c.JSON(http.StatusOK, assResp)
}
