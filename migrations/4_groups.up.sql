CREATE TABLE `groups` (
  `id` bigint PRIMARY KEY,
  `name` text NOT NULL,
  `currency` text NOT NULL,
  `simplify_debts` boolean NOT NULL DEFAULT false,
  `invite_code` text NOT NULL,
  `created_by` bigint NOT NULL REFERENCES `users`(`id`),
  UNIQUE(`invite_code`)
);

CREATE TABLE `group_members` (
  `group_id` bigint NOT NULL REFERENCES `groups`(`id`) ON DELETE CASCADE,
  `user_id` bigint NOT NULL REFERENCES `users`(`id`),
  PRIMARY KEY(`group_id`, `user_id`)
);

CREATE INDEX `group_members_user_idx` ON `group_members`(`user_id`);
