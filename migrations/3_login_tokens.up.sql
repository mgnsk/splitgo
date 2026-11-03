CREATE TABLE `login_tokens` (
  `token_hash` text PRIMARY KEY,
  `email` text NOT NULL,
  `created_at` bigint NOT NULL,
  `expires_at` bigint NOT NULL
);

CREATE INDEX `login_tokens_email_idx` ON `login_tokens`(`email`, `created_at`);
