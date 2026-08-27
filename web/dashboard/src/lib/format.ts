/** Currencies whose minor unit is the major unit — amounts are never scaled. */
const zeroDecimal = new Set([
	"BIF",
	"CLP",
	"DJF",
	"GNF",
	"JPY",
	"KMF",
	"KRW",
	"MGA",
	"PYG",
	"RWF",
	"UGX",
	"VND",
	"VUV",
	"XAF",
	"XOF",
	"XPF",
]);
/** Currencies with three minor digits rather than the usual two. */
const threeDecimal = new Set(["BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND"]);

function exponent(currency: string) {
	const code = currency.toUpperCase();
	if (zeroDecimal.has(code)) return 0;
	return threeDecimal.has(code) ? 3 : 2;
}

/**
 * formatAmount renders a minor-unit amount the way the API stores it. The exponent
 * table mirrors the server's, so a UGX 2500 reads as 2,500 rather than 25.00.
 */
export function formatAmount(minor: number, currency: string) {
	const digits = exponent(currency);
	const value = minor / 10 ** digits;
	try {
		return new Intl.NumberFormat(undefined, {
			style: "currency",
			currency,
			minimumFractionDigits: digits,
		}).format(value);
	} catch {
		// An unrecognized code is still worth showing; only the symbol is unavailable.
		return `${value.toFixed(digits)} ${currency}`;
	}
}

/** formatDateTime renders an API timestamp in the viewer's locale and zone. */
export function formatDateTime(value?: string) {
	if (!value) return "—";
	const date = new Date(value);
	return Number.isNaN(date.getTime()) ? value : date.toLocaleString();
}

/** formatRelative renders how long ago a timestamp was, for recency columns. */
export function formatRelative(value?: string) {
	if (!value) return "—";
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return value;
	const seconds = Math.round((date.getTime() - Date.now()) / 1000);
	const units: [Intl.RelativeTimeFormatUnit, number][] = [
		["day", 86400],
		["hour", 3600],
		["minute", 60],
	];
	const formatter = new Intl.RelativeTimeFormat(undefined, {
		numeric: "auto",
	});
	for (const [unit, size] of units) {
		if (Math.abs(seconds) >= size)
			return formatter.format(Math.round(seconds / size), unit);
	}
	return formatter.format(seconds, "second");
}

/** titleCase renders an API enum such as `super_admin` as `Super admin`. */
export function titleCase(value: string) {
	const spaced = value.replaceAll("_", " ");
	return spaced.charAt(0).toUpperCase() + spaced.slice(1);
}
