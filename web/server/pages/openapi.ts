import { assetUrl } from "../../../src/artifacts";

const icon = "https://assets.ohrats.party/assets/logo.092a1cece4d0.svg";

export function openapiReferencePage() {
  const css = assetUrl("openapi", "css"), script = assetUrl("openapi");
  if (!css || !script) return new Response("OpenAPI reference assets are unavailable", { status: 503 });
  return new Response(`<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <meta name="robots" content="noindex,nofollow">
  <title>OpenAPI Reference | RC</title>
  <link rel="icon" type="image/svg+xml" href="${icon}">
  <link rel="stylesheet" href="${css}">
</head>
<body>
  <div id="swagger-ui"></div>
  <script type="module" src="${script}"></script>
</body>
</html>`, {
    headers: { "content-type": "text/html; charset=utf-8", "cache-control": "public, max-age=0, must-revalidate" },
  });
}
