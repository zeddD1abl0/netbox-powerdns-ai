-- NetBox's zones, as the leader's refreshes last read them, so that every
-- replica's API serves their records, and a restarted one at once
-- (ADR-0048): one row for each zone in the server groups' views, the zone
-- as JSON, with its RRsets if it's active. A refresh writes a zone's row
-- only when its digest, the SHA-384 of that JSON, changes, and deletes it
-- once the zone has left the views. refresh_id is the refresh that wrote it
-- last, so that a replica reloads only the zones written since it last
-- loaded them. netbox_state is what the refreshes found of NetBox: status,
-- up or down, as the last refresh that tried NetBox found it, with error
-- why it was down, or empty before one did; and zones_as_of, when a full
-- refresh last read the zones, or empty before one did.
-- +goose Up
CREATE TABLE netbox_zones (
    view_name TEXT NOT NULL,
    name TEXT NOT NULL,
    refresh_id BIGINT NOT NULL,
    digest TEXT NOT NULL,
    zone_json TEXT NOT NULL,
    PRIMARY KEY (view_name, name)
);
CREATE INDEX netbox_zones_refresh ON netbox_zones (refresh_id);

CREATE TABLE netbox_state (
    singleton BIGINT PRIMARY KEY CHECK (singleton = 1),
    status TEXT NOT NULL,
    error TEXT NOT NULL,
    zones_as_of TEXT NOT NULL
);
INSERT INTO netbox_state (singleton, status, error, zones_as_of) VALUES (1, '', '', '');
