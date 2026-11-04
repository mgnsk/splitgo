-- IANA time zone detected from the user's browser, e.g. Europe/Tallinn.
-- Empty means unknown: the server's time zone is used.
ALTER TABLE `users` ADD COLUMN `timezone` text NOT NULL DEFAULT '';
