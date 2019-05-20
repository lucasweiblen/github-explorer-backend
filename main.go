package main

import (
	"log"
	"net/http"
	"os"
	"strconv"
	"teste/db"
	"teste/service"
	"teste/util"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
)

var (
	config *db.Config
)

type ErrorResponse map[string]interface{}

type jwtCustomClaims struct {
	Id       int    `json:id"`
	Username string `json:"username"`
	jwt.StandardClaims
}

func signUpHandler(c echo.Context) error {
	u := &db.User{}
	if err := c.Bind(u); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{"code": 1, "message": "Bad request!"})
	}
	hash, err := util.HashPassword(u.Password)
	if err != nil {
		log.Println("Error hashing password...")
		return c.JSON(http.StatusInternalServerError, ErrorResponse{"code": 2, "message": "Server error"})
	}
	u.Password = hash
	sqlStmt := `INSERT INTO users(username, password, email, created_on) VALUES ($1, $2, $3, $4) RETURNING id`
	err = config.Connection().QueryRow(sqlStmt, u.Username, u.Password, u.Email, "now()").Scan(&u.Id)
	if err != nil {
		log.Println(err)
		return c.JSON(http.StatusInternalServerError, ErrorResponse{
			"code":    3,
			"message": "SQL Error",
		})
	}
	log.Println("New user is id: ", u.Id)
	if err = service.SendEmail(u.Username, u.Email); err != nil {
		log.Println("Error sending welcome email!")
	}

	// generate JWT token and return on response
	return c.JSON(http.StatusOK, "OK")
}

func signinHandler(c echo.Context) error {
	u := &db.User{}
	if err := c.Bind(u); err != nil {
		// log this error
		return c.JSON(http.StatusBadRequest, ErrorResponse{"code": 1, "message": "Bad request!"})
	}
	user, err := db.ConfirmUser(config.Connection(), u)
	if err != nil {
		log.Println("Hash does not match")
		return c.JSON(http.StatusForbidden, ErrorResponse{"code": 4, "message": "Login credentials are not correct"})
	}
	claims := &jwtCustomClaims{
		user.Id,
		user.Username,
		jwt.StandardClaims{
			ExpiresAt: time.Now().Add(time.Hour * 24).Unix(),
		},
	}
	// Create token with claims
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)

	// Generate encoded token and send it as response.
	t, err := token.SignedString([]byte("secret"))
	if err != nil {
		return err
	}

	return c.JSON(http.StatusOK, echo.Map{
		"token": t,
		"user": map[string]interface{}{
			"id":                user.Id,
			"username":          user.Username,
			"languages":         user.Languages,
			"favorite_language": user.FavoriteLanguage,
			"frequency":         user.Frequency,
		},
	})
}

func getBookmarkedProjectsHandler(c echo.Context) error {
	// verify jwt token
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		log.Println("Problem converting id to integer")
		return c.JSON(http.StatusBadRequest, ErrorResponse{"code": 5, "message": "Bad request!"})
	}
	projects := db.FetchUserBookmarkedProjects(config.Connection(), userId)
	return c.JSON(http.StatusOK, projects)
}

func addBookmarkedProjectHandler(c echo.Context) error {
	// verify jwt token
	p := &db.Project{}
	userId, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		log.Println("Problem converting id to integer")
		return c.JSON(http.StatusBadRequest, ErrorResponse{"code": 5, "message": "Bad request!"})
	}
	if err := c.Bind(p); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{"code": 1, "message": "Bad request!"})
	}
	projectId, err := db.ProjectExists(config.Connection(), p.Name, p.Author, p.Language)
	if err != nil {
		newProjectId, err := db.AddProject(config.Connection(), p)
		if err != nil {
			log.Println("Problem saving new project!")
			return c.JSON(http.StatusInternalServerError,
				ErrorResponse{
					"code":    6,
					"message": "Failed to add new project",
				})
		}
		_, err = db.BookmarkProject(config.Connection(), userId, newProjectId)
		if err != nil {
			log.Println("Problems bookmarking project 1")
			return c.JSON(http.StatusInternalServerError,
				ErrorResponse{
					"code":    7,
					"message": "Failed to bookmark project",
				})
		}
		return c.JSON(http.StatusOK, "Bookmarked project 1")
	}
	_, err = db.BookmarkProject(config.Connection(), userId, projectId)
	if err != nil {
		log.Println("Problems bookmarking project 2")
		return c.JSON(http.StatusInternalServerError,
			ErrorResponse{
				"code":    8,
				"message": "Failed to bookmark project",
			})
	}
	return c.JSON(http.StatusOK, "Bookmarked project 2")
}

func main() {
	port, err := strconv.Atoi(os.Getenv("PORT"))
	if err != nil {
		log.Fatal(err)
	}
	config = db.NewConfig(
		os.Getenv("HOST"),
		port,
		os.Getenv("USER"),
		os.Getenv("PASS"),
		os.Getenv("DBNAME"))
	e := echo.New()
	e.Use(middleware.CORS())
	e.POST("/users", signUpHandler)
	e.POST("/users/login", signinHandler)
	e.GET("/users/:id/bookmarked_projects", getBookmarkedProjectsHandler)
	e.POST("/users/:id/bookmarked_projects", addBookmarkedProjectHandler)
	e.Logger.Fatal(e.Start(":1323"))
}
