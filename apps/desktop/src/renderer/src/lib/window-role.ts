export const windowRole = new URLSearchParams(window.location.search).get("window") === "main"
  ? "main" : "compact";
