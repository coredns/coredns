table "dns_rrs" {
  schema  = schema.corednsdb
  collate = "utf8mb4_bin"
  column "id" {
    null = false
    type = varchar(20)
  }
  column "created_at" {
    null = false
    type = datetime(6)
  }
  column "updated_at" {
    null = false
    type = datetime(6)
  }
  column "name" {
    null = false
    type = varchar(255)
  }
  column "rrtype" {
    null     = false
    type     = smallint
    unsigned = true
  }
  column "rrdata" {
    null = false
    type = longtext
  }
  column "class" {
    null     = false
    type     = smallint
    default  = 1
    unsigned = true
  }
  column "ttl" {
    null     = false
    type     = int
    default  = 3600
    unsigned = true
  }
  column "activated" {
    null    = false
    type    = bool
    default = 1
  }
  column "dns_zone_records" {
    null = false
    type = varchar(20)
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "dns_rrs_dns_zones_records" {
    columns     = [column.dns_zone_records]
    ref_columns = [table.dns_zones.column.id]
    on_update   = RESTRICT
    on_delete   = CASCADE
  }
  index "dnsrr_activated" {
    columns = [column.activated]
  }
  index "dnsrr_name_rrtype" {
    columns = [column.name, column.rrtype]
  }
  index "dns_rrs_dns_zones_records" {
    columns = [column.dns_zone_records]
  }
}
table "dns_zones" {
  schema  = schema.corednsdb
  collate = "utf8mb4_bin"
  column "id" {
    null = false
    type = varchar(20)
  }
  column "created_at" {
    null = false
    type = datetime(6)
  }
  column "updated_at" {
    null = false
    type = datetime(6)
  }
  column "name" {
    null = false
    type = varchar(255)
  }
  column "rrtype" {
    null     = false
    type     = smallint
    default  = 6
    unsigned = true
  }
  column "class" {
    null     = false
    type     = smallint
    default  = 1
    unsigned = true
  }
  column "ttl" {
    null     = false
    type     = int
    default  = 3600
    unsigned = true
  }
  column "ns" {
    null = false
    type = varchar(255)
  }
  column "mbox" {
    null = false
    type = varchar(255)
  }
  column "serial" {
    null     = false
    type     = int
    unsigned = true
  }
  column "refresh" {
    null     = false
    type     = int
    default  = 10800
    unsigned = true
  }
  column "retry" {
    null     = false
    type     = int
    default  = 3600
    unsigned = true
  }
  column "expire" {
    null     = false
    type     = int
    default  = 604800
    unsigned = true
  }
  column "minttl" {
    null     = false
    type     = int
    default  = 3600
    unsigned = true
  }
  column "activated" {
    null    = false
    type    = bool
    default = 1
  }
  primary_key {
    columns = [column.id]
  }
  index "dnszone_activated" {
    columns = [column.activated]
  }
  index "name" {
    unique  = true
    columns = [column.name]
  }
}
schema "corednsdb" {
  charset = "utf8mb4"
  collate = "utf8mb4_general_ci"
}
