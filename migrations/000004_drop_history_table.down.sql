CREATE TABLE IF NOT EXISTS history (
    id          serial PRIMARY KEY,
    source      varchar(255),
    destination varchar(255),
    original    varchar(255),
    translation varchar(255)
);
