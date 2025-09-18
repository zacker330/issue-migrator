package main

import (
	"fmt"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/issue-migrator/backend/handlers"
	"github.com/joho/godotenv"
)

func main() {

	if err := godotenv.Load(); err != nil {
		fmt.Println("[INFO] No .env file found")
	}

	// Keep Gin in debug mode to see all logs
	// gin.SetMode(gin.ReleaseMode)
	r := gin.Default()

	// Add global panic recovery middleware
	r.Use(gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		fmt.Printf("[PANIC] Recovered from panic: %v\n", recovered)
		c.JSON(500, gin.H{
			"error": fmt.Sprintf("Internal server error: %v", recovered),
		})
	}))

	config := cors.DefaultConfig()
	config.AllowOrigins = []string{"http://localhost:3000"}
	config.AllowMethods = []string{"GET", "POST", "OPTIONS"}
	config.AllowHeaders = []string{"Origin", "Content-Type", "Authorization"}
	r.Use(cors.New(config))

	api := r.Group("/api")
	{
		api.GET("/health", handlers.HealthCheck)
		api.POST("/github/issues", handlers.GetGitHubIssues)
		api.POST("/gitlab/issues", handlers.GetGitLabIssues)
		api.POST("/migrate", handlers.MigrateWithFiles) // Version with full file support
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	fmt.Printf("[SERVER] Starting on port %s\n", port)
	if err := r.Run(":" + port); err != nil {
		fmt.Printf("[ERROR] Failed to start server: %v\n", err)
		os.Exit(1)
	}
}
