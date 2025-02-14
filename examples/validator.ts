#!/usr/bin/env bun

function isValidEmail(email: string): boolean {
	return email.includes('@') && (email.match(/\./g) || []).length === 1;
}

const server = Bun.serve({
	port: 8823,
	async fetch(req) {

		const payload = await req.json();
		console.info(`Validating document: ${payload.pending?.model}/${payload.pending?.id}`);

		const val = payload.pending!.val as Record<string, unknown>;
		for (const email of val.to as Array<any>) {

			if (!isValidEmail(email)) {
				console.error(`Validation failed: invalid email '${email}'`);
				return new Response(JSON.stringify({
					reject: {
						message: `Invalid email address: ${email}`
					}
				}));
			}

		}

		console.info(`Validation passed`);
		return new Response(JSON.stringify({}));
	}
});

console.info(`Validator server listening on port ${server.port}`);
