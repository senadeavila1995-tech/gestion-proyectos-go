package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name         string             `bson:"name" json:"name"`
	Email        string             `bson:"email" json:"email"`
	PasswordHash string             `bson:"password_hash" json:"-"`
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
}

type RegisterInput struct {
	Name     string `json:"name" binding:"required,min=2,max=80"`
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,min=8,max=72"`
}

type LoginInput struct {
	Email    string `json:"email" binding:"required,email,max=254"`
	Password string `json:"password" binding:"required,min=1,max=72"`
}

type Claims struct {
	Email string `json:"email"`
	jwt.RegisteredClaims
}

var users *mongo.Collection
var jwtSecret []byte

func respondError(c *gin.Context, status int, message string) {
	c.JSON(status, gin.H{"error": message})
}

func createToken(user User) (string, error) {
	now := time.Now()
	claims := Claims{
		Email: user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   user.ID.Hex(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(24 * time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret)
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			respondError(c, http.StatusUnauthorized, "Token requerido")
			c.Abort()
			return
		}

		claims := &Claims{}
		token, err := jwt.ParseWithClaims(parts[1], claims, func(token *jwt.Token) (interface{}, error) {
			if token.Method != jwt.SigningMethodHS256 {
				return nil, errors.New("algoritmo JWT no permitido")
			}
			return jwtSecret, nil
		})

		if err != nil || !token.Valid {
			respondError(c, http.StatusUnauthorized, "Token inválido o vencido")
			c.Abort()
			return
		}

		id, err := primitive.ObjectIDFromHex(claims.Subject)
		if err != nil {
			respondError(c, http.StatusUnauthorized, "Identidad inválida")
			c.Abort()
			return
		}

		c.Set("userID", id)
		c.Next()
	}
}

func main() {
	_ = godotenv.Load()

	jwtSecret = []byte(os.Getenv("JWT_SECRET"))
	if len(jwtSecret) < 32 {
		log.Fatal("JWT_SECRET debe tener al menos 32 bytes. Configúralo en .env")
	}

	mongoURI := os.Getenv("MONGODB_URI")
	if mongoURI == "" {
		mongoURI = "mongodb://127.0.0.1:27017"
	}
	databaseName := os.Getenv("MONGODB_DATABASE")
	if databaseName == "" {
		databaseName = "gestion_proyectos_go"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8082"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		cancel()
		log.Fatalf("Error conectando con MongoDB: %v", err)
	}
	if err := client.Ping(ctx, nil); err != nil {
		cancel()
		_ = client.Disconnect(context.Background())
		log.Fatalf("MongoDB no responde: %v", err)
	}
	cancel()
	defer func() { _ = client.Disconnect(context.Background()) }()

	db := client.Database(databaseName)
	users = db.Collection("users")

	indexCtx, indexCancel := context.WithTimeout(context.Background(), 5*time.Second)
	_, err = users.Indexes().CreateOne(indexCtx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	indexCancel()
	if err != nil {
		log.Fatalf("No se pudo crear el índice único de email: %v", err)
	}

	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())

	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"service": "gestion-proyectos-go",
			"status":  "online",
			"version": "1.0.0",
		})
	})

	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "gestion-proyectos-go"})
	})

	router.GET("/api/ready", func(c *gin.Context) {
		pingCtx, pingCancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
		defer pingCancel()
		if err := db.RunCommand(pingCtx, bson.D{{Key: "ping", Value: 1}}).Err(); err != nil {
			respondError(c, http.StatusServiceUnavailable, "Base de datos no disponible")
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "ready", "database": "connected", "name": databaseName})
	})

	router.POST("/api/auth/register", func(c *gin.Context) {
		var input RegisterInput
		if err := c.ShouldBindJSON(&input); err != nil {
			respondError(c, http.StatusBadRequest, "Datos inválidos. Revisa nombre, correo y contraseña (8-72 caracteres).")
			return
		}

		input.Name = strings.TrimSpace(input.Name)
		input.Email = strings.ToLower(strings.TrimSpace(input.Email))
		if input.Name == "" {
			respondError(c, http.StatusBadRequest, "El nombre es obligatorio")
			return
		}

		hash, err := bcrypt.GenerateFromPassword([]byte(input.Password), bcrypt.DefaultCost)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "No se pudo proteger la contraseña")
			return
		}

		user := User{
			ID:           primitive.NewObjectID(),
			Name:         input.Name,
			Email:        input.Email,
			PasswordHash: string(hash),
			CreatedAt:    time.Now().UTC(),
		}

		insertCtx, insertCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer insertCancel()
		_, err = users.InsertOne(insertCtx, user)
		if mongo.IsDuplicateKeyError(err) {
			respondError(c, http.StatusConflict, "Ese correo ya está registrado")
			return
		}
		if err != nil {
			respondError(c, http.StatusInternalServerError, "No se pudo crear la cuenta")
			return
		}

		token, err := createToken(user)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "Cuenta creada, pero no se pudo generar el token")
			return
		}
		c.JSON(http.StatusCreated, gin.H{
			"message":      "Cuenta creada correctamente",
			"user":         gin.H{"id": user.ID.Hex(), "name": user.Name, "email": user.Email},
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   86400,
		})
	})

	router.POST("/api/auth/login", func(c *gin.Context) {
		var input LoginInput
		if err := c.ShouldBindJSON(&input); err != nil {
			respondError(c, http.StatusBadRequest, "Correo o contraseña inválidos")
			return
		}

		email := strings.ToLower(strings.TrimSpace(input.Email))
		queryCtx, queryCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer queryCancel()

		var user User
		err := users.FindOne(queryCtx, bson.M{"email": email}).Decode(&user)
		if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(input.Password)) != nil {
			respondError(c, http.StatusUnauthorized, "Correo o contraseña incorrectos")
			return
		}

		token, err := createToken(user)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "No se pudo generar el token")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":      "Inicio de sesión correcto",
			"user":         gin.H{"id": user.ID.Hex(), "name": user.Name, "email": user.Email},
			"access_token": token,
			"token_type":   "Bearer",
			"expires_in":   86400,
		})
	})

	private := router.Group("/api")
	private.Use(authMiddleware())
	private.GET("/auth/me", func(c *gin.Context) {
		id := c.MustGet("userID").(primitive.ObjectID)
		queryCtx, queryCancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer queryCancel()

		var user User
		if err := users.FindOne(queryCtx, bson.M{"_id": id}).Decode(&user); err != nil {
			respondError(c, http.StatusNotFound, "Usuario no encontrado")
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"user": gin.H{"id": user.ID.Hex(), "name": user.Name, "email": user.Email},
		})
	})

	registerProjectRoutes(router, db)

	log.Printf("API iniciada en http://127.0.0.1:%s", port)
	log.Printf("MongoDB conectado; base de datos: %s", databaseName)
	if err := router.Run(":" + port); err != nil {
		log.Fatalf("Error del servidor: %v", err)
	}
}
