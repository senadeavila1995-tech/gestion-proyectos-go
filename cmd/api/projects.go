package main

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type Project struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	OwnerID     primitive.ObjectID `bson:"owner_id" json:"-"`
	Title       string             `bson:"title" json:"title"`
	Description string             `bson:"description" json:"description"`
	Status      string             `bson:"status" json:"status"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

type Task struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	OwnerID     primitive.ObjectID `bson:"owner_id" json:"-"`
	ProjectID   primitive.ObjectID `bson:"project_id" json:"project_id"`
	Title       string             `bson:"title" json:"title"`
	Description string             `bson:"description" json:"description"`
	Status      string             `bson:"status" json:"status"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

type ProjectInput struct {
	Title       string `json:"title" binding:"required,max=120"`
	Description string `json:"description" binding:"max=2000"`
	Status      string `json:"status"`
}

type TaskInput struct {
	Title       string `json:"title" binding:"required,max=120"`
	Description string `json:"description" binding:"max=2000"`
	Status      string `json:"status"`
}

func requestUserID(c *gin.Context) primitive.ObjectID {
	return c.MustGet("userID").(primitive.ObjectID)
}

func validStatus(status string, allowed ...string) bool {
	for _, value := range allowed {
		if status == value {
			return true
		}
	}
	return false
}

func registerProjectRoutes(router *gin.Engine, db *mongo.Database) {
	projects := db.Collection("projects")
	tasks := db.Collection("tasks")

	private := router.Group("/api")
	private.Use(authMiddleware())

	// Crear proyecto.
	private.POST("/projects", func(c *gin.Context) {
		var input ProjectInput
		if err := c.ShouldBindJSON(&input); err != nil {
			respondError(c, http.StatusBadRequest, "Datos del proyecto inválidos")
			return
		}

		input.Title = strings.TrimSpace(input.Title)
		if input.Title == "" {
			respondError(c, http.StatusBadRequest, "El título es obligatorio")
			return
		}

		status := input.Status
		if status == "" {
			status = "active"
		}
		if !validStatus(status, "active", "completed") {
			respondError(c, http.StatusBadRequest, "Estado inválido: usa active o completed")
			return
		}

		now := time.Now().UTC()
		project := Project{
			ID:          primitive.NewObjectID(),
			OwnerID:     requestUserID(c),
			Title:       input.Title,
			Description: strings.TrimSpace(input.Description),
			Status:      status,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		if _, err := projects.InsertOne(ctx, project); err != nil {
			respondError(c, http.StatusInternalServerError, "No se pudo crear el proyecto")
			return
		}
		c.JSON(http.StatusCreated, gin.H{"project": project})
	})

	// Listar exclusivamente los proyectos del usuario autenticado.
	private.GET("/projects", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		cursor, err := projects.Find(ctx,
			bson.M{"owner_id": requestUserID(c)},
			options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}),
		)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "No se pudieron consultar los proyectos")
			return
		}
		defer cursor.Close(ctx)

		result := make([]Project, 0)
		if err := cursor.All(ctx, &result); err != nil {
			respondError(c, http.StatusInternalServerError, "Error leyendo los proyectos")
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": result, "count": len(result)})
	})

	// Consultar un proyecto propio.
	private.GET("/projects/:id", func(c *gin.Context) {
		id, err := primitive.ObjectIDFromHex(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID de proyecto inválido")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		var project Project
		err = projects.FindOne(ctx, bson.M{
			"_id": id, "owner_id": requestUserID(c),
		}).Decode(&project)
		if err != nil {
			respondError(c, http.StatusNotFound, "Proyecto no encontrado")
			return
		}
		c.JSON(http.StatusOK, gin.H{"project": project})
	})

	// Actualizar un proyecto propio.
	private.PUT("/projects/:id", func(c *gin.Context) {
		id, err := primitive.ObjectIDFromHex(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID de proyecto inválido")
			return
		}

		var input ProjectInput
		if err := c.ShouldBindJSON(&input); err != nil {
			respondError(c, http.StatusBadRequest, "Datos del proyecto inválidos")
			return
		}
		input.Title = strings.TrimSpace(input.Title)
		if input.Title == "" {
			respondError(c, http.StatusBadRequest, "El título es obligatorio")
			return
		}

		status := input.Status
		if status == "" {
			status = "active"
		}
		if !validStatus(status, "active", "completed") {
			respondError(c, http.StatusBadRequest, "Estado inválido: usa active o completed")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		result, err := projects.UpdateOne(ctx,
			bson.M{"_id": id, "owner_id": requestUserID(c)},
			bson.M{"$set": bson.M{
				"title":       input.Title,
				"description": strings.TrimSpace(input.Description),
				"status":      status,
				"updated_at":  time.Now().UTC(),
			}},
		)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "No se pudo actualizar el proyecto")
			return
		}
		if result.MatchedCount == 0 {
			respondError(c, http.StatusNotFound, "Proyecto no encontrado")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Proyecto actualizado"})
	})

	// Eliminar proyecto propio y sus tareas.
	private.DELETE("/projects/:id", func(c *gin.Context) {
		id, err := primitive.ObjectIDFromHex(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID de proyecto inválido")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		filter := bson.M{"_id": id, "owner_id": requestUserID(c)}
		result, err := projects.DeleteOne(ctx, filter)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "No se pudo eliminar el proyecto")
			return
		}
		if result.DeletedCount == 0 {
			respondError(c, http.StatusNotFound, "Proyecto no encontrado")
			return
		}
		_, _ = tasks.DeleteMany(ctx, bson.M{
			"project_id": id, "owner_id": requestUserID(c),
		})
		c.JSON(http.StatusOK, gin.H{"message": "Proyecto y tareas eliminados"})
	})

	// Crear tarea solo en un proyecto propio.
	private.POST("/projects/:id/tasks", func(c *gin.Context) {
		projectID, err := primitive.ObjectIDFromHex(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID de proyecto inválido")
			return
		}

		var input TaskInput
		if err := c.ShouldBindJSON(&input); err != nil {
			respondError(c, http.StatusBadRequest, "Datos de la tarea inválidos")
			return
		}
		input.Title = strings.TrimSpace(input.Title)
		if input.Title == "" {
			respondError(c, http.StatusBadRequest, "El título es obligatorio")
			return
		}

		status := input.Status
		if status == "" {
			status = "pending"
		}
		if !validStatus(status, "pending", "in_progress", "completed") {
			respondError(c, http.StatusBadRequest, "Estado inválido")
			return
		}

		ownerID := requestUserID(c)
		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()

		err = projects.FindOne(ctx, bson.M{
			"_id": projectID, "owner_id": ownerID,
		}).Err()
		if err != nil {
			respondError(c, http.StatusNotFound, "Proyecto no encontrado")
			return
		}

		now := time.Now().UTC()
		task := Task{
			ID:          primitive.NewObjectID(),
			OwnerID:     ownerID,
			ProjectID:   projectID,
			Title:       input.Title,
			Description: strings.TrimSpace(input.Description),
			Status:      status,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if _, err := tasks.InsertOne(ctx, task); err != nil {
			respondError(c, http.StatusInternalServerError, "No se pudo crear la tarea")
			return
		}
		c.JSON(http.StatusCreated, gin.H{"task": task})
	})

	// Listar tareas de un proyecto propio.
	private.GET("/projects/:id/tasks", func(c *gin.Context) {
		projectID, err := primitive.ObjectIDFromHex(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID de proyecto inválido")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		err = projects.FindOne(ctx, bson.M{
			"_id": projectID, "owner_id": requestUserID(c),
		}).Err()
		if err != nil {
			respondError(c, http.StatusNotFound, "Proyecto no encontrado")
			return
		}

		cursor, err := tasks.Find(ctx, bson.M{
			"project_id": projectID, "owner_id": requestUserID(c),
		}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
		if err != nil {
			respondError(c, http.StatusInternalServerError, "No se pudieron consultar las tareas")
			return
		}
		defer cursor.Close(ctx)

		result := make([]Task, 0)
		if err := cursor.All(ctx, &result); err != nil {
			respondError(c, http.StatusInternalServerError, "Error leyendo las tareas")
			return
		}
		c.JSON(http.StatusOK, gin.H{"results": result, "count": len(result)})
	})

	// Actualizar una tarea propia.
	private.PUT("/tasks/:id", func(c *gin.Context) {
		id, err := primitive.ObjectIDFromHex(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID de tarea inválido")
			return
		}

		var input TaskInput
		if err := c.ShouldBindJSON(&input); err != nil {
			respondError(c, http.StatusBadRequest, "Datos de la tarea inválidos")
			return
		}
		input.Title = strings.TrimSpace(input.Title)
		if input.Title == "" {
			respondError(c, http.StatusBadRequest, "El título es obligatorio")
			return
		}

		status := input.Status
		if status == "" {
			status = "pending"
		}
		if !validStatus(status, "pending", "in_progress", "completed") {
			respondError(c, http.StatusBadRequest, "Estado inválido")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		result, err := tasks.UpdateOne(ctx,
			bson.M{"_id": id, "owner_id": requestUserID(c)},
			bson.M{"$set": bson.M{
				"title":       input.Title,
				"description": strings.TrimSpace(input.Description),
				"status":      status,
				"updated_at":  time.Now().UTC(),
			}},
		)
		if err != nil {
			respondError(c, http.StatusInternalServerError, "No se pudo actualizar la tarea")
			return
		}
		if result.MatchedCount == 0 {
			respondError(c, http.StatusNotFound, "Tarea no encontrada")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Tarea actualizada"})
	})

	// Eliminar una tarea propia.
	private.DELETE("/tasks/:id", func(c *gin.Context) {
		id, err := primitive.ObjectIDFromHex(c.Param("id"))
		if err != nil {
			respondError(c, http.StatusBadRequest, "ID de tarea inválido")
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
		defer cancel()
		result, err := tasks.DeleteOne(ctx, bson.M{
			"_id": id, "owner_id": requestUserID(c),
		})
		if err != nil {
			respondError(c, http.StatusInternalServerError, "No se pudo eliminar la tarea")
			return
		}
		if result.DeletedCount == 0 {
			respondError(c, http.StatusNotFound, "Tarea no encontrada")
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Tarea eliminada"})
	})
}
