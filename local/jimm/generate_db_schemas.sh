#!/bin/bash

# This script generates database schemas from our running postgres deployment docker compose.
# Its purpose is to better inspect raw SQL generating our db models.

# Configuration
DB_CONTAINER="postgres"   
SCHEMA="public"           
OUTPUT_DIR="./db_schemas"

# Create output directory
mkdir -p $OUTPUT_DIR

# Get a list of tables in the specified schema, excluding system tables
TABLES=$(docker exec -i $DB_CONTAINER sh -c "
  psql -U \$POSTGRES_USER -d \$POSTGRES_DB -t -c \"
  SELECT tablename 
  FROM pg_tables 
  WHERE schemaname = '$SCHEMA' AND tablename NOT LIKE 'pg_%' AND tablename != 'information_schema';
  \""
)

# Loop through each table and dump its schema
for TABLE in $TABLES; do
  TABLE=$(echo $TABLE | xargs) # Trim whitespace
  if [[ ! -z "$TABLE" ]]; then
    echo "Extracting schema for table: $TABLE"
    docker exec -i $DB_CONTAINER sh -c "
      pg_dump -U \$POSTGRES_USER --schema-only --table=$SCHEMA.$TABLE \$POSTGRES_DB
    " | sed -E '
      # Remove the SET paragraph
      /SET /,/^$/d
      # Remove SEQUENCE paragraph
      /SEQUENCE/,/^$/d
      # Remove comments (lines starting with --)
      /^--/d
    ' | awk '
      # Replace multiple newlines with a single newline
      BEGIN {RS=""; ORS="\n\n"} 
      {print $0}
    ' > "$OUTPUT_DIR/${TABLE}_schema.sql"
  fi
done

echo "Schemas extracted to $OUTPUT_DIR."
