UPDATE navigation_categories
SET name = 'AegisAdmin',
    updated_at = strftime('%Y-%m-%dT%H:%M:%SZ', 'now')
WHERE name = 'Administration'
  AND id = (
      SELECT category_id
      FROM navigation_modules
      WHERE module_key = 'modules'
      LIMIT 1
  );
