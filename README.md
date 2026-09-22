# simple-image-processing-golang

A Go backend for user authentication and image processing. The API uses Gin for HTTP routing, PostgreSQL for account data, JWT for authentication, Azure Blob Storage for uploaded images, and the `imaging` package for image transformations.

## Features

- User signup with bcrypt password hashing
- User login with JWT creation
- Image upload to Azure Blob Storage
- Image transformations:
  - Resize
  - Crop
  - Rotate
  - Format conversion between PNG and JPEG
  - Invert, grayscale, flip, and blur filters

## Requirements

- Go `1.26.5` or newer
- PostgreSQL
- An Azure Storage account and blob container

## Configuration

Create a `.env` file in the project root:

```env
DB_LINK=postgres://username:password@localhost:5432/database_name
JWT_SECRET=replace-with-a-long-random-secret
CON_STRING=DefaultEndpointsProtocol=https;AccountName=...;AccountKey=...;EndpointSuffix=core.windows.net
CON_NAME=images
```

The application loads `.env` using `godotenv` and starts on port `3001`.

## Database

The current code expects an `account` table with `username` and `password` columns:

```sql
CREATE TABLE account (
    username TEXT PRIMARY KEY,
    password TEXT NOT NULL
);
```

Passwords should be stored as bcrypt hashes. Additional tables may be needed for image metadata as that functionality is developed.

## Run Locally

Install dependencies and start the server:

```bash
go mod download
go run ./cmd/expense-app
```

The API is available at:

```text
http://localhost:3001
```

## API Endpoints

### Sign up

`POST /signup`

Request body:

```json
{
  "username": "alice",
  "password": "your-password"
}
```

### Log in

`POST /login`

Request body:

```json
{
  "username": "alice",
  "password": "your-password"
}
```

A successful response contains a JWT:

```json
{
  "token": "your.jwt.token"
}
```

### Upload an image

`POST /images/add`

Send a `multipart/form-data` request with an `image` field. Supported file extensions are `.jpg`, `.jpeg`, and `.png`.

```bash
curl -X POST http://localhost:3001/images/add \
  -F "image=@./image.png"
```

### Apply an image transformation

`POST /images/transform`

Send the image and a `metadata` form field containing JSON. The `action` determines the transformation.

Resize example:

```bash
curl -X POST http://localhost:3001/images/transform \
  -F "image=@./image.png" \
  -F 'metadata={"action":"resize","metadata":{"ext":"png","resize":{"width":800,"height":600}}}' \
  --output resized.png
```

Crop example:

```json
{
  "action": "crop",
  "metadata": {
    "ext": "png",
    "resize": {
      "width": 1,
      "height": 1
    },
    "crop": {
      "x": 0,
      "y": 0,
      "width": 400,
      "height": 300
    }
  }
}
```

Rotate example:

```json
{
  "action": "rotate",
  "metadata": {
    "ext": "jpeg",
    "resize": {
      "width": 1,
      "height": 1
    },
    "rotate": 90
  }
}
```

Supported actions are `resize`, `crop`, `rotate`, `changeFormat`, and `filters`.

For filters, use this shape:

```json
{
  "action": "filters",
  "metadata": {
    "ext": "png",
    "resize": {
      "width": 1,
      "height": 1
    },
    "filters": {
      "inverted": false,
      "grayscale": true,
      "flip": false,
      "blur": 2
    }
  }
}
```

## Project Structure

```text
cmd/expense-app/       Application entry point
 database/             PostgreSQL connection helper
 internals/router/     Gin route registration
 internals/services/   HTTP handlers for auth and images
 middleware/           JWT authentication middleware
 pkg/                  JWT and image-processing helpers
 images/               Local image files used during development
```

## Authentication

JWT validation is implemented in `middleware.AuthHandler`, but the current router does not yet attach this middleware to any route. When authentication is applied to a route, clients can send either of these headers:

```text
Authorization: Bearer <token>
```

or:

```text
authorization: <token>
```

The current router registers the authentication middleware but does not yet attach it to the image routes. Review this before exposing the API publicly.

## Development Notes

- Never commit `.env` files or storage credentials.
- Use parameterized SQL queries for database writes and reads.
- The `GET /images/:id` route is registered but does not currently have a handler.
- Test Azure Storage and PostgreSQL configuration before deploying.

## License

No license has been specified for this project yet.
