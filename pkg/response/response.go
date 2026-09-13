package response

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

func Success(c *gin.Context, data any) {
	c.JSON(http.StatusOK, Envelope{Code: 0, Data: data})
}

// Error reports a readable business failure. Transport failures use HTTPError.
func Error(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusOK, Envelope{Code: -1, Msg: message})
}

func Fail(c *gin.Context, message string) {
	Error(c, message)
}

func HTTPError(c *gin.Context, status int) {
	c.AbortWithStatusJSON(status, Envelope{Code: status, Msg: http.StatusText(status)})
}
