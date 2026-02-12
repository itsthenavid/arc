// Atlas configuration for Arc (local dev).
// Run from infra/db/atlas for deterministic relative paths.

variable "atlas_url" {
  type    = string
  default = getenv("ARC_ATLAS_URL") != "" ? getenv("ARC_ATLAS_URL") : getenv("ARC_DATABASE_URL")
}

variable "atlas_dev_url" {
  type    = string
  // Always isolate dev DB. Never default to the real DB.
  default = getenv("ARC_ATLAS_DEV_URL") != "" ? getenv("ARC_ATLAS_DEV_URL") : "docker://postgres/16/dev"
}

env "local" {
  url = var.atlas_url
  dev = var.atlas_dev_url

  schema {
    src = "file://schema.sql"
  }
}
