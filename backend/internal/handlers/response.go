package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data"`
	Error   interface{} `json:"error"`
}

func OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, APIResponse{Success: true, Data: data, Error: nil})
}

func Fail(c *gin.Context, status int, message string) {
	c.JSON(status, APIResponse{Success: false, Data: nil, Error: message})
}
