-- +goose Up
-- +goose StatementBegin
CREATE INDEX IF NOT EXISTS movies_title_idx On movies USING GIN (to_tsvector('simple', title));
CREATE INDEX IF NOT EXISTS movies_genres_idx ON movies USING GIN (genres);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX IF EXISTS movies_title_idx;
DROP INDEX IF EXISTS movies_genres_idx;
-- +goose StatementEnd
