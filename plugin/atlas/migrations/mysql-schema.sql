-- Create "dns_zones" table
CREATE TABLE `dns_zones` (
    `id` varchar(20) NOT NULL,
    `created_at` datetime(6) NOT NULL,
    `updated_at` datetime(6) NOT NULL,
    `name` varchar(255) NOT NULL,
    `rrtype` smallint unsigned NOT NULL DEFAULT 6,
    `class` smallint unsigned NOT NULL DEFAULT 1,
    `ttl` int unsigned NOT NULL DEFAULT 3600,
    `ns` varchar(255) NOT NULL,
    `mbox` varchar(255) NOT NULL,
    `serial` int unsigned NOT NULL,
    `refresh` int unsigned NOT NULL DEFAULT 10800,
    `retry` int unsigned NOT NULL DEFAULT 3600,
    `expire` int unsigned NOT NULL DEFAULT 604800,
    `minttl` int unsigned NOT NULL DEFAULT 3600,
    `activated` bool NOT NULL DEFAULT 1,
    PRIMARY KEY (`id`),
    INDEX `dnszone_activated` (`activated`),
    UNIQUE INDEX `name` (`name`)
) CHARSET utf8mb4 COLLATE utf8mb4_bin;

-- Create "dns_rrs" table
CREATE TABLE `dns_rrs` (
    `id` varchar(20) NOT NULL,
    `created_at` datetime(6) NOT NULL,
    `updated_at` datetime(6) NOT NULL,
    `name` varchar(255) NOT NULL,
    `rrtype` smallint unsigned NOT NULL,
    `rrdata` longtext NOT NULL,
    `class` smallint unsigned NOT NULL DEFAULT 1,
    `ttl` int unsigned NOT NULL DEFAULT 3600,
    `activated` bool NOT NULL DEFAULT 1,
    `dns_zone_records` varchar(20) NOT NULL,
    PRIMARY KEY (`id`),
    INDEX `dns_rrs_dns_zones_records` (`dns_zone_records`),
    INDEX `dnsrr_activated` (`activated`),
    INDEX `dnsrr_name_rrtype` (`name`, `rrtype`),
    CONSTRAINT `dns_rrs_dns_zones_records` FOREIGN KEY (`dns_zone_records`) REFERENCES `dns_zones` (`id`) ON UPDATE NO ACTION ON DELETE CASCADE
) CHARSET utf8mb4 COLLATE utf8mb4_bin;