import type { APIEnvelope } from "./types.js";

export class MomobaseAPIError extends Error {
	readonly status: number;
	readonly code: string;
	readonly body: unknown;

	constructor(status: number, code: string, message: string, body: unknown) {
		super(message);
		this.name = "MomobaseAPIError";
		this.status = status;
		this.code = code;
		this.body = body;
	}

	static async fromResponse(response: Response): Promise<MomobaseAPIError> {
		let body: unknown;
		try {
			body = await response.json();
		} catch {
			body = await response.text().catch(() => "");
		}
		const maybe = body as Partial<APIEnvelope>;
		const code = maybe.error?.code ?? `HTTP_${response.status}`;
		const message =
			maybe.error?.message ?? maybe.message ?? response.statusText;
		return new MomobaseAPIError(response.status, code, message, body);
	}
}
