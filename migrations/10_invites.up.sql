-- Invite emails sent by users, for rate limiting.
CREATE TABLE `invites` (
  `user_id` bigint NOT NULL REFERENCES `users`(`id`) ON DELETE CASCADE,
  `created_at` bigint NOT NULL
);

CREATE INDEX `invites_user_idx` ON `invites`(`user_id`, `created_at`);
