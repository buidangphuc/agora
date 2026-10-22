const formatter = new Intl.DateTimeFormat("en-GB", {
	timeZone: "Asia/Ho_Chi_Minh",
	day: "2-digit",
	month: "2-digit",
});

export function formatDeliveryDate(
	iso: string | null | undefined,
): string | null {
	if (!iso) return null;
	const d = new Date(iso);
	if (Number.isNaN(d.getTime())) return null;
	const parts = formatter.formatToParts(d);
	const day = parts.find((p) => p.type === "day")?.value;
	const month = parts.find((p) => p.type === "month")?.value;
	return day && month ? `${day}/${month}` : null;
}
