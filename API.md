# API Reference

The server listens on port `3001` by default. Transformation requests use
`multipart/form-data`; their `metadata` field is a JSON string.

## Authentication

### Sign up

`POST /signup` with `Content-Type: application/json`:

```json
{
  "username": "alice",
  "password": "your-password"
}
```

### Log in

`POST /login` with `Content-Type: application/json`:

```json
{
  "username": "alice",
  "password": "your-password"
}
```

## Upload an image

### `POST /images/add`

Send an `image` file in a `multipart/form-data` request. Supported file types
are `.jpg`, `.jpeg`, and `.png`. An optional `payload` field can provide a user
ID:

```json
{
  "userid": "alice"
}
```

Example:

```bash
curl -X POST http://localhost:3001/images/add \
  -F "image=@./image.png" \
  -F 'payload={"userid":"alice"}'
```

## Transform images

### `POST /images/transform`

Send two multipart fields:

- `image`: the source image.
- `metadata`: JSON in the request format below.

The batch request format below replaces the old single-action
`{"action": "...", "metadata": {...}}` format.

Every successful transform response is a ZIP archive named `results.zip`.
Each generated image gets **one** edit, selected round-robin from `edits`, and
that edit is applied to the original image. For example, two edits and 50
results produce 25 images from each edit. Random ranges are sampled separately
for every result. Outputs are named `image_001.png`, `image_002.png`, etc.

### Common request format

```json
{
  "count": 10,
  "ext": "png",
  "edits": [
    {
      "action": "rotate",
      "range": { "min": -5, "max": 5 }
    }
  ],
  "flip": {
    "enabled": true,
    "probability": 0.25
  }
}
```

- `count`: required, 1–100.
- `ext`: required output format: `png`, `jpg`, or `jpeg`.
- `edits`: required and non-empty. `count` must be at least the number of edits.
- `flip`: optional additional vertical flip applied independently to each
  result after its selected edit. When enabled, `probability` is required and
  must be from `0` to `1` (for example, `0.25` means 25%). Omit `flip` or set
  `enabled` to `false` to disable it.

### Edit request examples

Each example below is an edit object to put in the `edits` array.

**Resize** — set one dimension to `0` to preserve aspect ratio:

```json
{
  "action": "resize",
  "resize": { "width": 800, "height": 0 }
}
```

**Random crop** — `width` and `height` ranges are fractions of the source
dimensions, independently sampled per result. The crop position is also random.
Fractions must be from `0.01` to `1`.

```json
{
  "action": "randomCrop",
  "randomCrop": {
    "width": { "min": 0.7, "max": 1.0 },
    "height": { "min": 0.7, "max": 1.0 }
  }
}
```

**Random scale** — resize the full image by a randomly selected factor. Output
dimensions can differ from the source. Scale must be from `0.1` to `4`.

```json
{
  "action": "randomScale",
  "range": { "min": 0.8, "max": 1.2 }
}
```

**Rotate** — degrees, from `-360` to `360`:

```json
{
  "action": "rotate",
  "range": { "min": -5, "max": 5 }
}
```

**Hue** — degrees, from `-360` to `360`:

```json
{
  "action": "hue",
  "range": { "min": -30, "max": 30 }
}
```

**Exposure** — stops, from `-10` to `10`:

```json
{
  "action": "exposure",
  "range": { "min": -1, "max": 1 }
}
```

**Brightness** — percentage adjustment, from `-100` to `100`:

```json
{
  "action": "brightness",
  "range": { "min": -20, "max": 20 }
}
```

**Contrast** — percentage adjustment, from `-100` to `100`:

```json
{
  "action": "contrast",
  "range": { "min": -20, "max": 20 }
}
```

**Saturation** — percentage adjustment, from `-100` to `100`:

```json
{
  "action": "saturation",
  "range": { "min": -30, "max": 30 }
}
```

**Gaussian blur** — sigma, from `0` to `100`; `0` means no blur:

```json
{
  "action": "blur",
  "range": { "min": 0.5, "max": 3 }
}
```

**Gaussian noise** — standard deviation in 8-bit RGB levels, from `0` to `255`:

```json
{
  "action": "gaussianNoise",
  "range": { "min": 5, "max": 30 }
}
```

**Change output format for this edit** — `png`, `jpg`, or `jpeg`:

```json
{
  "action": "changeFormat",
  "format": "jpeg"
}
```

### Multiple edits and optional flip example

```json
{
  "count": 50,
  "ext": "png",
  "edits": [
    {
      "action": "rotate",
      "range": { "min": -5, "max": 5 }
    },
    {
      "action": "randomCrop",
      "randomCrop": {
        "width": { "min": 0.7, "max": 1.0 },
        "height": { "min": 0.7, "max": 1.0 }
      }
    },
    {
      "action": "brightness",
      "range": { "min": -10, "max": 10 }
    }
  ],
  "flip": {
    "enabled": true,
    "probability": 0.5
  }
}
```

The edit allocation is 17 rotate, 17 random crop, and 16 brightness results.
Each result additionally has a 50% independent chance of vertical flipping.

### Example curl request

```bash
curl -X POST http://localhost:3001/images/transform \
  -F "image=@./image.png" \
  -F 'metadata={"count":10,"ext":"png","edits":[{"action":"randomScale","range":{"min":0.8,"max":1.2}},{"action":"saturation","range":{"min":-20,"max":20}}],"flip":{"enabled":true,"probability":0.25}}' \
  --output results.zip
```
