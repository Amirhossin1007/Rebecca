import { type FetchOptions, $fetch as ohMyFetch } from "ofetch";
import { getAuthToken } from "utils/authStorage";

export const $fetch = ohMyFetch.create({
	baseURL: import.meta.env.VITE_BASE_API,
});

export const fetcher = <T = any>(
	url: string,
	ops: FetchOptions<"json"> = {},
) => {
	const token = getAuthToken();
	const method = String(ops.method || "GET").toUpperCase();
	if (token) {
		ops.headers = {
			...(ops?.headers || {}),
			Authorization: `Bearer ${getAuthToken()}`,
		};
	}
	if (method === "GET") {
		ops.cache = "no-store";
	}
	return $fetch<T>(url, ops);
};

export const fetch = fetcher;
