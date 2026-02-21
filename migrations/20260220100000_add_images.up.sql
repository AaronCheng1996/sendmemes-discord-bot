CREATE TABLE IF NOT EXISTS images (
    id serial PRIMARY KEY,
    url text NOT NULL,
    source text,
    guild_id text,
    created_at timestamptz DEFAULT now()
);

INSERT INTO images (url, source) VALUES (
    '/assets/sample/image.png',
    'default'
);
