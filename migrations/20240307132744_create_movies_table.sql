-- +goose Up
-- +goose StatementBegin
CREATE TABLE IF NOT EXISTS movies (
			id bigserial PRIMARY KEY,
			created_at timestamp(0) with time zone NOT NULL DEFAULT NOW(),
			title text NOT NULL,
			year integer NOT NULL,
			runtime integer NOT NULL,
			genres text[] NOT NULL,
			version integer NOT NULL DEFAULT 1
		);

ALTER TABLE movies ADD CONSTRAINT movies_runtime_check CHECK (runtime >= 0);
ALTER TABLE movies ADD CONSTRAINT movies_year_check CHECK (year BETWEEN 1888 AND date_part('year', now()));
ALTER TABLE movies ADD CONSTRAINT genres_length_check CHECK (array_length(genres, 1) BETWEEN 1 AND 5);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
 ALTER TABLE movies DROP CONSTRAINT IF EXISTS movies_runtime_check;
    
ALTER TABLE movies DROP CONSTRAINT IF EXISTS movies_year_check;

ALTER TABLE movies DROP CONSTRAINT IF EXISTS genres_length_check;

DROP TABLE IF EXISTS movies;
-- +goose StatementEnd
