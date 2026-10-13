# Gestión de Proyectos API

API REST para gestionar proyectos y tareas, desarrollada con Go, Gin y MongoDB.

## Tecnologías

- Go
- Gin
- MongoDB
- JWT para autenticación
- bcrypt para el hash de contraseñas

## Funcionalidades

- Registro e inicio de sesión.
- Autenticación mediante JWT.
- Consulta del perfil autenticado.
- CRUD de proyectos.
- Creación, consulta, actualización y eliminación de tareas.
- Aislamiento de proyectos y tareas por usuario.
- Validación de datos y respuestas HTTP.

## Requisitos

- Go 1.22 o superior.
- MongoDB en ejecución.

## Configuración local

1. Clona o descarga el proyecto.
2. Copia `.env.example` a `.env`.
3. Configura la URI de MongoDB, el nombre de la base de datos y un secreto JWT aleatorio.
4. Instala las dependencias y ejecuta la API:

```bash
go mod download
go run ./cmd/api
```

La configuración local se carga desde `.env`. No publiques ese archivo ni compartas el secreto JWT.

## Endpoints

| Método | Endpoint | Acceso |
|---|---|---|
| GET | `/` | Público |
| GET | `/api/health` | Público |
| GET | `/api/ready` | Público |
| POST | `/api/auth/register` | Público |
| POST | `/api/auth/login` | Público |
| GET | `/api/auth/me` | JWT |
| POST | `/api/projects` | JWT |
| GET | `/api/projects` | JWT |
| GET | `/api/projects/:id` | JWT |
| PUT | `/api/projects/:id` | JWT |
| DELETE | `/api/projects/:id` | JWT |
| POST | `/api/projects/:id/tasks` | JWT |
| GET | `/api/projects/:id/tasks` | JWT |
| PUT | `/api/tasks/:id` | JWT |
| DELETE | `/api/tasks/:id` | JWT |

Para rutas protegidas, envía el encabezado `Authorization: Bearer <token>`.

## Estados

- Proyectos: `active`, `completed`.
- Tareas: `pending`, `in_progress`, `completed`.

## Validación realizada

Se comprobaron manualmente el registro, el inicio de sesión, el acceso al perfil con JWT, el rechazo de correos duplicados, el CRUD de proyectos y tareas y el aislamiento entre usuarios.

## Seguridad

Las contraseñas se almacenan mediante bcrypt. Las rutas privadas requieren JWT y las operaciones sobre proyectos y tareas filtran por el propietario autenticado.

Este proyecto es una API en desarrollo; antes de desplegarlo en producción se deben añadir pruebas automatizadas, documentación OpenAPI y controles adicionales de seguridad.
