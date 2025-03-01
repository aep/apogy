#!/usr/bin/env bun

import { DefaultService, ValidationRequest, Document, OpenAPI } from '../api/ts/client';
OpenAPI.BASE = 'http://localhost:27666';

const server = Bun.serve({
	port: 8823,
	async fetch(req) {

		const payload = await req.json() as ValidationRequest;
		console.info(`Unrolling: ${payload.pending?.model}/${payload.pending?.id}`);


		if (payload.current?.val?.unrolled) {
			console.info(`was already unrolled, not doing anything`);
			return new Response(JSON.stringify({}));
		}

		const val = payload.pending!.val as Record<string, unknown>;
		for (const email of val.to as Array<any>) {


			const nudoc: Document = {
				id: payload.pending?.id + "@" + email,
				model: 'com.example.Email',
				val: {
					newsletter: payload.pending?.id,
					to: email,
				}
			};

			console.log("creating ", nudoc.model, nudoc.id)

			DefaultService.putDocument(nudoc)

		}

		console.info(`done unrolling`);
		return new Response(JSON.stringify({}));
	}
});

console.info(`Validator server listening on port ${server.port}`);
