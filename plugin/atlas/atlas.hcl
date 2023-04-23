// Define an environment named "mysql"
env "mysql" {
  // Declare where the schema definition resides.
  // Also supported: ["file://multi.hcl", "file://schema.hcl"].
  src = "file://migrations/schema.hcl"

  // Define the URL of the database which is managed
  // in this environment.
  url = "mysql://core:secret@localhost:3306/corednsdb"

  // Define the URL of the Dev Database for this environment
  // See: https://atlasgo.io/concepts/dev-database
  // dev = "docker://mysql/8/schema"
}

env "mariadb" {
  src = "file://migrations/schema.hcl"
  url = "mariadb://core:secret@localhost:3307/corednsdb"
  # dev = "docker://mysql/8/schema"
}

env "postgres" {
  src = "file://migrations/postgres/schema.hcl"
  url = "postgres://core:secret@localhost:5432/corednsdb?sslmode=disable"
}
