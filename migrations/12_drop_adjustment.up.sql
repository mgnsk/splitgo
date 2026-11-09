-- The adjustment split type was removed: keep the computed shares as exact amounts.
UPDATE `expense_participants` SET `split_value` = ''
WHERE `expense_id` IN (SELECT `id` FROM `expenses` WHERE `split_type` = 'adjustment');
UPDATE `expenses` SET `split_type` = 'exact' WHERE `split_type` = 'adjustment';
