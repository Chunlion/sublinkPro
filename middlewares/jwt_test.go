package middlewares

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthTokenRejectsMissingOrMalformedTokenWithUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, token := range []string{"", "invalid"} {
		t.Run(token, func(t *testing.T) {
			router := gin.New()
			router.Use(AuthToken)
			router.GET("/protected", func(c *gin.Context) { c.Status(http.StatusOK) })

			request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/protected", nil)
			if token != "" {
				request.Header.Set("Authorization", token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)

			if response.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusUnauthorized)
			}
			var body struct {
				Code int `json:"code"`
			}
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != http.StatusUnauthorized {
				t.Fatalf("body code = %d, want %d", body.Code, http.StatusUnauthorized)
			}
		})
	}
}
