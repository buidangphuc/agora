import { Card } from "@/components/ui/Card";
import { Descriptions } from "@/components/ui/Descriptions";
import { SelectedSku, SelectedStock } from "./SelectedSku";

/**
 * "Chi tiết" section (`#specs`, always rendered): a Descriptions specs block from
 * real fields only (category name, selected-variant stock and SKU) followed by the
 * description text. A row without data is omitted, never invented.
 */
export function SpecsSection({
  categoryName,
  hasSku,
  description,
}: {
  categoryName?: string;
  /** Whether any variant carries a SKU (the value itself is the client leaf). */
  hasSku: boolean;
  description: string;
}) {
  const items = [
    ...(categoryName
      ? [{ key: "category", label: "Danh mục", children: categoryName }]
      : []),
    { key: "stock", label: "Kho hàng", children: <SelectedStock /> },
    ...(hasSku
      ? [{ key: "sku", label: "Mã SKU", children: <SelectedSku /> }]
      : []),
  ];

  return (
    <section
      id="specs"
      aria-labelledby="specs-heading"
      className="scroll-mt-40"
    >
      <Card padded>
        <div className="space-y-6">
          <h2
            id="specs-heading"
            className="text-xl font-semibold tracking-tight text-text-primary"
          >
            CHI TIẾT SẢN PHẨM
          </h2>
          <Descriptions items={items} column={2} />
          {description.trim() !== "" && (
            <div className="space-y-3 border-t border-border-subtle pt-6">
              <h3 className="text-base font-semibold text-text-primary">
                MÔ TẢ SẢN PHẨM
              </h3>
              <p className="whitespace-pre-line break-words text-base leading-relaxed text-text-secondary">
                {description}
              </p>
            </div>
          )}
        </div>
      </Card>
    </section>
  );
}
