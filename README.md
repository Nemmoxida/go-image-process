# simple-image-processing-golang

A Go backend for user authentication and image processing. The API uses Gin
for HTTP routing, PostgreSQL for account data, JWT for authentication, Azure
Blob Storage for uploaded images, and the `imaging` package for transformations.

## Features

- User signup with bcrypt password hashing and login with JWT creation
- Image upload to Azure Blob Storage
- Image transformations: resize, rotate, hue, exposure, brightness, contrast,
  saturation, Gaussian blur and noise, random crop, random scale, and optional
  probabilistic vertical flip
- Batch image generation with an even distribution across selected edit types
- Optional YOLO annotation validation and synchronized updates for geometric
  image transformations
- PNG and JPEG format conversion

## Requirements

- Go `1.26.5` or newer
- PostgreSQL
- An Azure Storage account and blob container

## Configuration

Create a `.env` file in the project root:

```env
DB_LINK=******localhost:5432/database_name
JWT_SECRET=replace-with-a-long-random-secret
CON_STRING=DefaultEndpointsProtocol=https;AccountName=...;AccountKey=...;EndpointSuffix=core.windows.net
CON_NAME=images
```

The application loads `.env` using `godotenv` and starts on port `3001`.

## Database

The current code expects an `account` table with `username` and `password`
columns:

```sql
CREATE TABLE account (
    username TEXT PRIMARY KEY,
    password TEXT NOT NULL
);
```

Passwords should be stored as bcrypt hashes. Additional tables may be needed
for image metadata as that functionality is developed.

## Run Locally

```bash
go mod download
go run ./cmd/expense-app
```

The API is available at `http://localhost:3001`. See [API.md](./API.md) for all
request body examples and transformation options.

## Project Structure

```text
cmd/expense-app/       Application entry point
database/              PostgreSQL connection helper
internals/router/      Gin route registration
internals/services/    HTTP handlers for auth and images
middleware/            JWT authentication middleware
pkg/                   JWT and image-processing helpers
images/                Local image files used during development
```

## Authentication

JWT validation is implemented in `middleware.AuthHandler`, but the current
router does not attach this middleware to any route. Review this before
exposing the API publicly.

## Development Notes

- Never commit `.env` files or storage credentials.
- Use parameterized SQL queries for database writes and reads.
- The `GET /images/:id` route is registered but does not currently have a handler.
- Test Azure Storage and PostgreSQL configuration before deploying.

## License

No license has been specified for this project yet.
