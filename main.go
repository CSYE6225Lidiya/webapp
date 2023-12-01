package main

import (
	"app/assignment/controllers"
	"app/assignment/models"
	"encoding/csv"
	"errors"
	"fmt"
	"io/ioutil"
	"strings"
	"time"

	"net/http"
	"os"
	"strconv"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/sns"
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
var snsArn string

var db *gorm.DB
var dbErr error

type DbConfig struct {
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	DB       string `yaml:"db"`
	SnsArn   string `yaml:"snsarn"`
}
type AssignmentData struct {
	Name string `json:"name"`
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
			//	log.Info().Msg("")
			dbUser = dbconfig.User
			dbPassword = dbconfig.Password
			dbName = dbconfig.DB
			dbHost = dbconfig.Host
			dbPort = dbconfig.Port
			snsArn = dbconfig.SnsArn
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
	db.AutoMigrate(&models.Account{}, &models.Assignment{}, &models.Submission{})

	//file, err := os.Open("./config/users.csv") // Windows
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

	router.POST("/v1/assignments/:id/submission", submitAssignment)

	router.Run()

}

func healthCheck(c *gin.Context) {

	// Increment the counter metric every time the API is hit
	statsdClient.Increment("healthz_counter")

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
	dbHealth, dbConnErr := gorm.Open(mysql.Open(dbConn), &gorm.Config{})
	fmt.Printf("DB Connection Status: error=%v", dbConnErr)
	sqlDB, err := dbHealth.DB()
	if err != nil {
		log.Error().Err(err).Msg("Healthz Endpoint:Unable to create a sql database object")
	}
	defer sqlDB.Close()

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

	// Increment the counter metric every time the API is hit
	statsdClient.Increment("createassignment_counter")

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

	// Increment the counter metric every time the API is hit
	statsdClient.Increment("getallassignments_counter")

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

	// Increment the counter metric every time the API is hit
	statsdClient.Increment("getanassignment_counter")

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

	// Increment the counter metric every time the API is hit
	statsdClient.Increment("deleteassignment_counter")

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

	// Increment the counter metric every time the API is hit
	statsdClient.Increment("updateassignment_counter")

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

func submitAssignment(c *gin.Context) {

	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("X-Content-Type-Options", "nosniff")

	log.Info().Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("SubmitAssignment Endpoint")

	// Authenticate the user and obtain their user ID
	userID, err := controllers.AuthenticateUser(c, db)
	if err != nil {
		err := errors.New("AUTHENTICATION ERROR")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("SubmitAssignment Endpoint:Unable to authenticate the request")
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Authorization Failed"})
		return
	}

	// Get the assignment ID from the request parameters
	assignmentIDStr := c.Param("id")
	assignmentID, err := strconv.ParseUint(assignmentIDStr, 10, 64)
	if err != nil {
		err := errors.New("INVALID ASSIGNMENT ID")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("SubmitAssignment Endpoint:The assignment ID is Invalid")
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid assignment ID"})
		return
	}

	// Check if the assignment exists
	var assignment models.Assignment
	if err := db.Where("id = ?", assignmentID).First(&assignment).Error; err != nil {
		err := errors.New("ASSIGNMENT NOT FOUND")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("SubmitAssignment Endpoint:The assignment doesn't exist")
		c.JSON(http.StatusNotFound, gin.H{"error": "Assignment not found"})
		return
	}

	// Validate Req Body contains URL
	var submissionInput models.SubmissionInput
	if err := c.ShouldBindJSON(&submissionInput); err != nil {
		err := errors.New("INCORRECT REQUEST BODY")
		log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("SubmitAssignment Endpoint:The request body is incorrect")
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Check if submission already exists for the given assignment ID
	var existingSubmission models.Submission
	//result := db.Where("assignment_id = ?", assignmentID).First(&existingSubmission)

	// SNS Client Creation
	snsClient := createSNSSession()
	//topicArn := "arn:aws:sns:us-east-1:203689115380:topiceast"
	topicArn := snsArn

	result := db.Where("assignment_id = ? AND account_id = ?", assignmentID, userID).First(&existingSubmission)
	if result.RowsAffected > 0 { // Submission already exists
		println("**************Submission exists")
		// Compare retries
		if assignment.NoOfAttempts == existingSubmission.SubmissionRetries {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Maximum no of attempts reached! No more retries available"})
			return
		}

		//Compare date

		// Parse the deadline string into a time.Time object
		deadlineTime, err := time.Parse("2006-01-02T15:04:05.999Z", assignment.Deadline)

		if err != nil {
			println("------------------------------DATE PARSE ERR", err.Error())
			c.JSON(400, gin.H{"error": "Error parsing deadline date"})
			return
		}
		currentTime := time.Now().UTC()
		// Compare the current time with the assignment deadline
		if currentTime.After(deadlineTime) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Assignment deadline has passed"})
			return
		}

		existingSubmission.SubmissionRetries++
		existingSubmission.SubmissionUrl = submissionInput.SubmissionUrl
		// Save the updated assignment to the database
		if err := db.Save(&existingSubmission).Error; err != nil {
			err := errors.New("UPDATE ERROR")
			log.Error().Err(err).Str("ip", c.ClientIP()).Str("http_method", c.Request.Method).Msg("SubmitAssignment Endpoint:Failed to update the assignment")
			c.JSON(http.StatusExpectationFailed, gin.H{"error": "Failed to update the assignment submission"})
			return
		}

		subResp := models.SubmissionResponse{
			ID:                existingSubmission.ID,
			AssignmentID:      assignment.ID,
			SubmissionUrl:     submissionInput.SubmissionUrl,
			SubmissionDate:    existingSubmission.UpdatedAt.String(),
			SubmissionRetries: existingSubmission.SubmissionRetries,
		}

		c.JSON(http.StatusOK, subResp)

		assName := assignment.Name
		retry := strconv.Itoa(subResp.SubmissionRetries)
		subTime := currentTime.String()
		userEmail, _, _ := c.Request.BasicAuth()
		subEmail := userEmail
		downloadURL := subResp.SubmissionUrl
		var uName string

		// Split the email address by "@" to separate the username and domain
		parts := strings.Split(subEmail, "@")

		// Check if the split resulted in two parts
		if len(parts) == 2 {
			// The username is the first part before "@"
			uName = parts[0]

			// Print the result
			fmt.Println(uName)
		}

		message1 := fmt.Sprintf(`{"name": "%s", "age": "two", "assName": "%s", "retry": "%s", "email": "%s", "time": "%s", "downloadURL": "%s"}`, uName, assName, retry, subEmail, subTime, downloadURL)
		fmt.Println("******************************")
		fmt.Println(message1)
		fmt.Println("******************************")
		//message := `{"name": "John","age":"two"}`

		//snsClient := createSNSSession()

		err = publishToSNS(snsClient, topicArn, message1)
		if err != nil {
			fmt.Print("***************************PUBLISH ERRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRR", err.Error())
			log.Error().Err(err).Msg(err.Error())
			//	return
		} else {
			fmt.Print("FMT___________________PUBLISHED SUCCESSFULLY TO SNS Resubmission")
			log.Info().Msg("Published successfully to sns from Resubmission")
		}

		return

	} else {
		println("*submssn dosent exist-----Create new submission")
		// Compare date
		// Parse the deadline string into a time.Time object
		deadlineTime, err := time.Parse("2006-01-02T15:04:05.999Z", assignment.Deadline)

		if err != nil {
			println("------------------------------DATE PARSE ERR", err.Error())
			c.JSON(400, gin.H{"error": "Error parsing deadline date"})
			return
		}
		currentTime := time.Now().UTC()
		// Compare the current time with the assignment deadline
		if currentTime.After(deadlineTime) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Assignment deadline has passed"})
			return
		}

		// Create a submission

		newSubmission := models.Submission{
			AssignmentID:      assignmentID,
			AccountID:         userID,
			SubmissionUrl:     submissionInput.SubmissionUrl,
			SubmissionRetries: 1, // Set other fields as needed
		}

		db.Create(&newSubmission)

		subResp := models.SubmissionResponse{
			ID:                newSubmission.ID,
			AssignmentID:      assignment.ID,
			SubmissionUrl:     submissionInput.SubmissionUrl,
			SubmissionDate:    newSubmission.UpdatedAt.String(),
			SubmissionRetries: 1,
		}

		c.JSON(http.StatusOK, subResp)

		// // Publish to SNS
		// cfg, err := config.LoadDefaultConfig(context.TODO())
		// if err != nil {
		// 	fmt.Println("$$$$$$$$$$$$$$$$$$$$$$AWSConfigReadErr", err)
		// }
		// client := sns.NewFromConfig(cfg)
		// //message := fmt.Sprintf("New submission for Assignment ID %d by User ID %d. Submission URL: %s", assignmentID, userID, submissionInput.SubmissionUrl)
		// // Construct the AssignmentData struct
		// // assignmentData := AssignmentData{Name: assignment.Name}
		// // // Convert AssignmentData to JSON
		// // messageBody, err := json.Marshal(assignmentData)
		// // msgStr := string(messageBody)
		// // if err != nil {
		// // 	fmt.Println("$$$$$$$$$$JSONMARSHALERRMSGBODY", err)
		// // }

		// topicArn := "arn:aws:sns:us-east-1:203689115380:topiceast"

		// // publishInput := &sns.PublishInput{
		// // 	TopicArn:         &topicArn, // Replace with your actual SNS topic ARN
		// // 	Message:          aws.String(msgStr),
		// // 	MessageStructure: aws.String("json"),
		// // }

		// // _, err = client.Publish(context.TODO(), publishInput)
		// // fmt.Println("$$$$$$$$$$$$$$$SNSPublishErr", err)

		// message := `{"name": "John","age":"two"}`
		// // Publish the message to the SNS topic
		// publishInput := &sns.PublishInput{
		// 	Message:  aws.String(message),
		// 	TopicArn: aws.String(topicArn),
		// }

		// _, err = client.Publish(context.TODO(), publishInput)
		// fmt.Println("$$$$$$$$$$$$$$$SNSPublishErr", err)
		// if err != nil {
		// 	fmt.Println("$$$$$$$$$$$$$$$$$$$$$$$$$Error publishing message:", err.Error())
		// 	//return
		// }

		//---------------------------------------------------------------------------------------------------------------
		//	topicArn := "arn:aws:sns:us-east-1:203689115380:topiceast" // replace with your SNS topic ARN
		//message := `{"name": "John","age":"two"}`

		//snsClient := createSNSSession()

		assName := assignment.Name
		retry := strconv.Itoa(subResp.SubmissionRetries)
		subTime := currentTime.String()
		userEmail, _, _ := c.Request.BasicAuth()
		subEmail := userEmail
		downloadURL := subResp.SubmissionUrl
		var uName string

		// Split the email address by "@" to separate the username and domain
		parts := strings.Split(subEmail, "@")

		// Check if the split resulted in two parts
		if len(parts) == 2 {
			// The username is the first part before "@"
			uName = parts[0]

			// Print the result
			fmt.Println(uName)
		}

		message1 := fmt.Sprintf(`{"name": "%s", "age": "two", "assName": "%s", "retry": "%s", "email": "%s", "time": "%s", "downloadURL": "%s"}`, uName, assName, retry, subEmail, subTime, downloadURL)
		fmt.Println("******************************")
		fmt.Println(message1)
		fmt.Println("******************************")

		err = publishToSNS(snsClient, topicArn, message1)
		if err != nil {
			fmt.Print("***************************PUBLISH ERRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRRR", err.Error())
			log.Error().Err(err).Msg(err.Error())
			//	return
		} else {
			fmt.Print("FMT___________________PUBLISHED SUCCESSFULLY TO SNS NewSubmission")
			log.Info().Msg("Published successfully to sns NewSubmission")
		}

		return
	}

}

func createSNSSession() *sns.SNS {
	fmt.Println("************INSIDE CREATE NEW SESSION")
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String("us-east-1"), // replace with your AWS region
	}))

	return sns.New(sess)
}

func publishToSNS(snsClient *sns.SNS, topicArn, message string) error {
	fmt.Println("************PUBLISH TO SNS")
	params := &sns.PublishInput{
		Message:  aws.String(message),
		TopicArn: aws.String(topicArn),
	}

	_, err := snsClient.Publish(params)
	return err
}
