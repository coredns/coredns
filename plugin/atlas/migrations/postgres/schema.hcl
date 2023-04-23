table "dns_rrs" {
  schema = schema.public
  column "id" {
    null = false
    type = character_varying(20)
  }
  column "created_at" {
    null = false
    type = timestamptz
  }
  column "updated_at" {
    null = false
    type = timestamptz
  }
  column "name" {
    null = false
    type = character_varying(255)
  }
  column "rrtype" {
    null = false
    type = smallint
  }
  column "rrdata" {
    null = false
    type = text
  }
  column "class" {
    null    = false
    type    = smallint
    default = 1
  }
  column "ttl" {
    null    = false
    type    = integer
    default = 3600
  }
  column "activated" {
    null    = false
    type    = boolean
    default = true
  }
  column "dns_zone_records" {
    null = false
    type = character_varying(20)
  }
  primary_key {
    columns = [column.id]
  }
  foreign_key "dns_rrs_dns_zones_records" {
    columns     = [column.dns_zone_records]
    ref_columns = [table.dns_zones.column.id]
    on_update   = NO_ACTION
    on_delete   = CASCADE
  }
  index "dnsrr_activated" {
    columns = [column.activated]
  }
  index "dnsrr_name_rrtype" {
    columns = [column.name, column.rrtype]
  }
}
table "dns_zones" {
  schema = schema.public
  column "id" {
    null = false
    type = character_varying(20)
  }
  column "created_at" {
    null = false
    type = timestamptz
  }
  column "updated_at" {
    null = false
    type = timestamptz
  }
  column "name" {
    null = false
    type = character_varying(255)
  }
  column "rrtype" {
    null    = false
    type    = smallint
    default = 6
  }
  column "class" {
    null    = false
    type    = smallint
    default = 1
  }
  column "ttl" {
    null    = false
    type    = integer
    default = 3600
  }
  column "ns" {
    null = false
    type = character_varying(255)
  }
  column "mbox" {
    null = false
    type = character_varying(255)
  }
  column "serial" {
    null = false
    type = integer
  }
  column "refresh" {
    null    = false
    type    = integer
    default = 10800
  }
  column "retry" {
    null    = false
    type    = integer
    default = 3600
  }
  column "expire" {
    null    = false
    type    = integer
    default = 604800
  }
  column "minttl" {
    null    = false
    type    = integer
    default = 3600
  }
  column "activated" {
    null    = false
    type    = boolean
    default = true
  }
  primary_key {
    columns = [column.id]
  }
  index "dns_zones_name_key" {
    unique  = true
    columns = [column.name]
  }
  index "dnszone_activated" {
    columns = [column.activated]
  }
}
schema "public" {
}
