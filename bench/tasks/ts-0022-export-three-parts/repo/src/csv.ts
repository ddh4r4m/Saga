export const HEADER = ["sku", "name", "qty", "updated_at"];

// One CSV row. Fields are written as they are; the catalogue used to have
// only plain names.
export function toCsvRow(fields: string[]): string {
  return fields.join(",");
}
