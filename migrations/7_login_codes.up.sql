-- One-time codes allow signing in from an installed app (PWA), which does not
-- share cookies with the browser that opens emailed links.
ALTER TABLE `login_tokens` ADD COLUMN `code_hash` text NOT NULL DEFAULT '';
ALTER TABLE `login_tokens` ADD COLUMN `attempts` integer NOT NULL DEFAULT 0;

CREATE TABLE `login_failures` (
  `email` text NOT NULL,
  `created_at` bigint NOT NULL
);

CREATE INDEX `login_failures_email_idx` ON `login_failures`(`email`, `created_at`);
