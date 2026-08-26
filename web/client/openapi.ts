import { SwaggerUIBundle } from "swagger-ui-dist";
import "swagger-ui-dist/swagger-ui.css";
import "../openapi.css";

SwaggerUIBundle({
  dom_id: "#swagger-ui",
  url: "/api/v1/openapi/json",
  deepLinking: true,
  displayRequestDuration: true,
  docExpansion: "list",
  defaultModelsExpandDepth: -1,
  validatorUrl: null,
  supportedSubmitMethods: [],
});
