const STORAGE_PREFIX = "rb-tutorials";
export const TUTORIALS_UPDATED_EVENT = "rb-tutorials-updated";

export const normalizeTutorialLang = (lang?: string | null) => {
	const normalized = (lang || "en").toLowerCase();
	return normalized.startsWith("fa") ? "fa" : "en";
};

export const getTutorialAssetUrl = (lang?: string | null) => {
	const normalized = normalizeTutorialLang(lang);
	const file = normalized === "fa" ? "totfa.json" : "toten.json";
	const base = (import.meta.env.BASE_URL || "/").replace(/\/$/, "");
	return `${base}/statics/locles/${file}`;
};

const getStorageKeys = (lang: string) => ({
	updated: `${STORAGE_PREFIX}-last-updated-${lang}`,
	menuIds: `${STORAGE_PREFIX}-menu-ids-${lang}`,
	unseen: `${STORAGE_PREFIX}-unseen-ids-${lang}`,
});

export const readTutorialStorage = (lang: string) => {
	if (typeof window === "undefined") {
		return {
			updated: null as string | null,
			ids: [] as string[],
			unseen: [] as string[],
		};
	}
	const keys = getStorageKeys(lang);
	const updated = window.localStorage.getItem(keys.updated);
	const rawIds = window.localStorage.getItem(keys.menuIds);
	const rawUnseen = window.localStorage.getItem(keys.unseen);
	let ids: string[] = [];
	if (rawIds) {
		try {
			const parsed = JSON.parse(rawIds);
			if (Array.isArray(parsed)) {
				ids = parsed.filter((id) => typeof id === "string");
			}
		} catch {
			ids = [];
		}
	}
	let unseen: string[] = [];
	if (rawUnseen) {
		try {
			const parsed = JSON.parse(rawUnseen);
			if (Array.isArray(parsed)) {
				unseen = parsed.filter((id) => typeof id === "string");
			}
		} catch {
			unseen = [];
		}
	}
	return { updated, ids, unseen };
};

export const writeTutorialStorage = (
	lang: string,
	updated: string,
	ids: string[],
	unseen: string[],
) => {
	if (typeof window === "undefined") return;
	const keys = getStorageKeys(lang);
	window.localStorage.setItem(keys.updated, updated);
	window.localStorage.setItem(keys.menuIds, JSON.stringify(ids));
	window.localStorage.setItem(keys.unseen, JSON.stringify(unseen));
	window.dispatchEvent(new CustomEvent(TUTORIALS_UPDATED_EVENT));
};

export const isTutorialUpdated = (
	currentUpdated?: string | null,
	storedUpdated?: string | null,
) =>
	Boolean(currentUpdated && storedUpdated && currentUpdated !== storedUpdated);

export const syncTutorialUpdateStorage = (
	lang: string,
	currentUpdated: string | undefined | null,
	currentIds: string[],
) => {
	const stored = readTutorialStorage(lang);
	const activeUnseen = stored.unseen.filter((id) => currentIds.includes(id));
	const nextUpdated = currentUpdated || stored.updated || "";
	const hasCurrentSnapshot = stored.updated !== null && stored.ids.length > 0;
	const unique = (values: string[]) => Array.from(new Set(values));

	if (!hasCurrentSnapshot) {
		if (nextUpdated || currentIds.length > 0) {
			writeTutorialStorage(lang, nextUpdated, currentIds, []);
		}
		return [] as string[];
	}

	const newIds = currentIds.filter((id) => !stored.ids.includes(id));
	const hasVersionChange = Boolean(
		nextUpdated && stored.updated && nextUpdated !== stored.updated,
	);
	const nextIds = unique([...stored.ids, ...currentIds]);
	const nextUnseen = hasVersionChange
		? unique([...activeUnseen, ...newIds])
		: activeUnseen;
	const shouldWrite =
		nextUpdated !== (stored.updated || "") ||
		nextIds.length !== stored.ids.length ||
		activeUnseen.length !== stored.unseen.length ||
		hasVersionChange;

	if (shouldWrite) {
		writeTutorialStorage(lang, nextUpdated, nextIds, nextUnseen);
	}

	return nextUnseen;
};

export const acknowledgeTutorialIds = (
	lang: string,
	idsToAcknowledge: string | string[],
) => {
	if (typeof window === "undefined") return;
	const ids = Array.isArray(idsToAcknowledge)
		? idsToAcknowledge
		: [idsToAcknowledge];
	const stored = readTutorialStorage(lang);
	if (!stored.unseen.length) return;
	const nextUnseen = stored.unseen.filter((id) => !ids.includes(id));
	if (nextUnseen.length === stored.unseen.length) return;
	writeTutorialStorage(lang, stored.updated || "", stored.ids, nextUnseen);
};
