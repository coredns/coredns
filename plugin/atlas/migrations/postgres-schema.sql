-- Add new schema named "public"
CREATE SCHEMA IF NOT EXISTS "public";

-- Create "dns_zones" table
CREATE TABLE "public"."dns_zones" (
    "id" character varying(20) NOT NULL,
    "created_at" timestamptz NOT NULL,
    "updated_at" timestamptz NOT NULL,
    "name" character varying(255) NOT NULL,
    "rrtype" smallint NOT NULL DEFAULT 6,
    "class" smallint NOT NULL DEFAULT 1,
    "ttl" integer NOT NULL DEFAULT 3600,
    "ns" character varying(255) NOT NULL,
    "mbox" character varying(255) NOT NULL,
    "serial" integer NOT NULL,
    "refresh" integer NOT NULL DEFAULT 10800,
    "retry" integer NOT NULL DEFAULT 3600,
    "expire" integer NOT NULL DEFAULT 604800,
    "minttl" integer NOT NULL DEFAULT 3600,
    "activated" boolean NOT NULL DEFAULT true,
    PRIMARY KEY ("id")
);

-- Create index "dns_zones_name_key" to table: "dns_zones"
CREATE UNIQUE INDEX "dns_zones_name_key" ON "public"."dns_zones" ("name");

-- Create index "dnszone_activated" to table: "dns_zones"
CREATE INDEX "dnszone_activated" ON "public"."dns_zones" ("activated");

-- Create "dns_rrs" table
CREATE TABLE "public"."dns_rrs" (
    "id" character varying(20) NOT NULL,
    "created_at" timestamptz NOT NULL,
    "updated_at" timestamptz NOT NULL,
    "name" character varying(255) NOT NULL,
    "rrtype" smallint NOT NULL,
    "rrdata" text NOT NULL,
    "class" smallint NOT NULL DEFAULT 1,
    "ttl" integer NOT NULL DEFAULT 3600,
    "activated" boolean NOT NULL DEFAULT true,
    "dns_zone_records" character varying(20) NOT NULL,
    PRIMARY KEY ("id"),
    CONSTRAINT "dns_rrs_dns_zones_records" FOREIGN KEY ("dns_zone_records") REFERENCES "public"."dns_zones" ("id") ON UPDATE NO ACTION ON DELETE CASCADE
);

-- Create index "dnsrr_activated" to table: "dns_rrs"
CREATE INDEX "dnsrr_activated" ON "public"."dns_rrs" ("activated");

-- Create index "dnsrr_name_rrtype" to table: "dns_rrs"
CREATE INDEX "dnsrr_name_rrtype" ON "public"."dns_rrs" ("name", "rrtype");