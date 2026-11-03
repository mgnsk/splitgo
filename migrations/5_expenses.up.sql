CREATE TABLE `expenses` (
  `id` bigint PRIMARY KEY,
  `group_id` bigint NOT NULL REFERENCES `groups`(`id`) ON DELETE CASCADE,
  `kind` text NOT NULL CHECK (`kind` IN ('expense', 'settlement')),
  `description` text NOT NULL,
  -- Total amount in minor units of currency.
  `amount` bigint NOT NULL CHECK (`amount` > 0),
  `currency` text NOT NULL,
  `split_type` text NOT NULL,
  `paid_by` bigint NOT NULL REFERENCES `users`(`id`),
  `added_by` bigint NOT NULL REFERENCES `users`(`id`),
  -- Date of the expense as YYYY-MM-DD.
  `date` text NOT NULL,
  `updated_at` bigint NOT NULL,
  `deleted_at` bigint
);

CREATE INDEX `expenses_group_idx` ON `expenses`(`group_id`, `date` DESC, `id` DESC);

CREATE TABLE `expense_participants` (
  `expense_id` bigint NOT NULL REFERENCES `expenses`(`id`) ON DELETE CASCADE,
  `user_id` bigint NOT NULL REFERENCES `users`(`id`),
  -- Share of the total in minor units.
  `share` bigint NOT NULL CHECK (`share` >= 0),
  -- Original split input (percentage, shares, exact or adjustment amount).
  `split_value` text NOT NULL DEFAULT '',
  PRIMARY KEY(`expense_id`, `user_id`)
);

CREATE INDEX `expense_participants_user_idx` ON `expense_participants`(`user_id`);
