// Atlas configuration for Arc (local dev).
// Run from infra/db/atlas for deterministic relative paths.

variable "atlas_url" {
  type    = string
  default = getenv("ARC_ATLAS_URL") != "" ? getenv("ARC_ATLAS_URL") : getenv("ARC_DATABASE_URL")
}

variable "atlas_dev_url" {
  type = string
  // Prefer explicit dev url, then atlas_url, then database_url.
  default = getenv("ARC_ATLAS_DEV_URL") != "" ? getenv("ARC_ATLAS_DEV_URL") : (
    getenv("ARC_ATLAS_URL") != "" ? getenv("ARC_ATLAS_URL") : getenv("ARC_DATABASE_URL")
  )
}

env "local" {
  url = var.atlas_url
  dev = var.atlas_dev_url

  schema {
    // Relative to this atlas.hcl directory.
    src = "file://schema.sql"
  }
}
