package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
)

func TestHealthCheckEndpoint(t *testing.T) {

	// Gin router for testing
	router := gin.Default()
	router.GET("/healthz", healthCheck)

	// Create a test HTTP request to the "/health" endpoint
	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		t.Fatal(err)
	}

	// Create a test HTTP recorder to capture the response
	w := httptest.NewRecorder()

	// Serve the HTTP request
	router.ServeHTTP(w, req)

	// Check the response status code
	assert.Equal(t, http.StatusOK, w.Code)
}
