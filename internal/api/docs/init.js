// nbpdns's API reference (ADR-0034): Scalar, reading the API's own OpenAPI
// document, with everything that would reach another host turned off. The
// page's Content-Security-Policy refuses such requests anyway.
Scalar.createApiReference("#app", {
  url: "openapi.yaml",
  // Inter and JetBrains Mono come from Scalar's CDN; the browser's own fonts
  // are used instead.
  withDefaultFonts: false,
  // Agent Scalar would upload the spec to Scalar's hosted service.
  agent: { disabled: true },
  mcp: { disabled: true },
  telemetry: false,
  showDeveloperTools: "never",
  hideClientButton: false,
});
