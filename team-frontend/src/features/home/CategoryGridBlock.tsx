import { Card, CardHeader, CardTitle } from "@/components/ui/Card";
import { CategoryBar } from "@/features/listing/CategoryBar";
import { listCategories } from "@/lib/gateway/listings";

/** Category tiles; renders nothing when the categories call fails or is empty. */
export async function CategoryGridBlock() {
  const categories = await listCategories().catch(() => []);
  if (categories.length === 0) return null;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="text-lg">Danh mục ngành hàng</CardTitle>
      </CardHeader>
      <CategoryBar categories={categories} variant="grid" />
    </Card>
  );
}
