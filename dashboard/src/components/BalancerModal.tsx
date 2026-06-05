import {
	Box,
	Button,
	FormControl,
	FormErrorMessage,
	FormHelperText,
	FormLabel,
	HStack,
	Input,
	Modal,
	ModalCloseButton,
	ModalOverlay,
	Select,
	Tag,
	TagCloseButton,
	TagLabel,
	Text,
	VStack,
	Wrap,
	WrapItem,
} from "@chakra-ui/react";
import { type FC, useEffect, useMemo, useState } from "react";
import { useForm } from "react-hook-form";
import { useTranslation } from "react-i18next";
import { SearchableTagSelect } from "./common/SearchableTagSelect";
import {
	XrayDialogSection,
	XrayModalBody,
	XrayModalContent,
	XrayModalFooter,
	XrayModalHeader,
} from "./xray/XrayDialog";

export type BalancerFormValues = {
	tag: string;
	strategy: string;
	selector: string[];
	fallbackTag: string;
};

interface BalancerModalProps {
	isOpen: boolean;
	onClose: () => void;
	mode: "create" | "edit";
	initialBalancer?: BalancerFormValues | null;
	outboundTags: string[];
	excludedOutboundTags?: string[];
	existingTags: string[];
	onSubmit: (values: BalancerFormValues) => void;
}

const DEFAULT_BALANCER: BalancerFormValues = {
	tag: "",
	strategy: "random",
	selector: [],
	fallbackTag: "",
};

const parseTags = (value: string) =>
	value
		.split(/[\s,]+/)
		.map((item) => item.trim())
		.filter(Boolean);

const uniq = (values: string[]) => Array.from(new Set(values));
const normalizeBalancerOutboundTag = (tag: string) => tag.trim().toLowerCase();

export const BalancerModal: FC<BalancerModalProps> = ({
	isOpen,
	onClose,
	mode,
	initialBalancer,
	outboundTags,
	excludedOutboundTags = [],
	existingTags,
	onSubmit,
}) => {
	const { t } = useTranslation();
	const [selectorInput, setSelectorInput] = useState("");

	const modalForm = useForm<BalancerFormValues>({
		defaultValues: DEFAULT_BALANCER,
	});

	useEffect(() => {
		modalForm.register("selector");
	}, [modalForm]);

	const tagValue = modalForm.watch("tag");
	const rawSelectorValue = modalForm.watch("selector") ?? [];
	const rawFallbackTagValue = modalForm.watch("fallbackTag") ?? "";
	const excludedOutboundTagKeys = useMemo(
		() =>
			new Set(
				excludedOutboundTags.map(normalizeBalancerOutboundTag).filter(Boolean),
			),
		[excludedOutboundTags],
	);
	const isExcludedBalancerOutbound = (tag: string) => {
		const normalized = normalizeBalancerOutboundTag(tag);
		return normalized === "blocked" || excludedOutboundTagKeys.has(normalized);
	};
	const selectableOutboundTags = useMemo(
		() => outboundTags.filter((tag) => !isExcludedBalancerOutbound(tag)),
		[excludedOutboundTagKeys, outboundTags],
	);
	const selectorValue = rawSelectorValue.filter(
		(tag) => !isExcludedBalancerOutbound(tag),
	);
	const fallbackTagValue = isExcludedBalancerOutbound(rawFallbackTagValue)
		? ""
		: rawFallbackTagValue;
	const normalizedTag = tagValue.trim();
	const duplicateTag = !normalizedTag || existingTags.includes(normalizedTag);
	const emptySelector = selectorValue.length === 0;

	useEffect(() => {
		if (!isOpen) return;
		modalForm.reset(
			initialBalancer
				? {
						...DEFAULT_BALANCER,
						...initialBalancer,
						tag: initialBalancer.tag ?? "",
						selector: (initialBalancer.selector ?? []).filter(
							(tag) => !isExcludedBalancerOutbound(tag),
						),
						fallbackTag: isExcludedBalancerOutbound(
							initialBalancer.fallbackTag ?? "",
						)
							? ""
							: initialBalancer.fallbackTag ?? "",
					}
				: DEFAULT_BALANCER,
		);
		setSelectorInput("");
	}, [excludedOutboundTagKeys, initialBalancer, isOpen, modalForm]);

	const addSelectorTags = (value: string) => {
		const tags = parseTags(value).filter(
			(tag) => !isExcludedBalancerOutbound(tag),
		);
		if (tags.length === 0) return;
		const merged = uniq([...(selectorValue ?? []), ...tags]);
		modalForm.setValue("selector", merged, { shouldDirty: true });
	};

	const removeSelectorTag = (tag: string) => {
		modalForm.setValue(
			"selector",
			(selectorValue ?? []).filter((item) => item !== tag),
			{ shouldDirty: true },
		);
	};

	const onSubmitInternal = modalForm.handleSubmit((data) => {
		if (!isValid) return;
		const payload: BalancerFormValues = {
			tag: data.tag.trim(),
			strategy: data.strategy,
			selector: uniq(
				data.selector
					.map((item) => item.trim())
					.filter((item) => item && !isExcludedBalancerOutbound(item)),
			),
			fallbackTag: isExcludedBalancerOutbound(data.fallbackTag ?? "")
				? ""
				: data.fallbackTag ?? "",
		};
		onSubmit(payload);
	});

	const isValid = useMemo(
		() => !duplicateTag && !emptySelector,
		[duplicateTag, emptySelector],
	);

	return (
		<Modal isOpen={isOpen} onClose={onClose} size="2xl" scrollBehavior="inside">
			<ModalOverlay bg="blackAlpha.400" backdropFilter="blur(8px)" />
			<XrayModalContent mx="3">
				<XrayModalHeader>
					{mode === "edit"
						? t("pages.xray.balancer.editBalancer")
						: t("pages.xray.balancer.addBalancer")}
				</XrayModalHeader>
				<ModalCloseButton />
				<form onSubmit={onSubmitInternal}>
					<XrayModalBody>
						<VStack spacing={3} align="stretch">
							<XrayDialogSection title={t("pages.xray.balancer.addBalancer")}>
								<VStack spacing={4} align="stretch">
									<FormControl isInvalid={duplicateTag}>
										<FormLabel>{t("pages.xray.balancer.tag")}</FormLabel>
										<Input
											{...modalForm.register("tag")}
											size="sm"
											placeholder={t("pages.xray.balancer.tagDesc")}
										/>
										{duplicateTag ? (
											<FormErrorMessage>
												{t("pages.xray.balancer.tagError")}
											</FormErrorMessage>
										) : (
											<FormHelperText>
												{t("pages.xray.balancer.tagDesc")}
											</FormHelperText>
										)}
									</FormControl>
									<FormControl>
										<FormLabel>
											{t("pages.xray.balancer.balancerStrategy")}
										</FormLabel>
										<Select {...modalForm.register("strategy")} size="sm">
											{["random", "roundRobin", "leastLoad", "leastPing"].map(
												(s) => (
													<option key={s} value={s}>
														{s}
													</option>
												),
											)}
										</Select>
									</FormControl>
									<FormControl isInvalid={emptySelector}>
										<FormLabel>
											{t("pages.xray.balancer.balancerSelectors")}
										</FormLabel>
										<VStack align="stretch" spacing={2}>
											{selectableOutboundTags.length > 0 && (
												<SearchableTagSelect
													mode="multiple"
													options={selectableOutboundTags}
													value={selectorValue}
													onChange={(value) =>
														modalForm.setValue("selector", value as string[], {
															shouldDirty: true,
														})
													}
													placeholder={t(
														"pages.xray.balancer.selectOutbound",
														"Select outbound tag",
													)}
													searchPlaceholder={t("search", "Search")}
													emptyText={t(
														"pages.xray.outbound.empty",
														"No outbound found",
													)}
												/>
											)}
											<HStack>
												<Input
													size="sm"
													value={selectorInput}
													onChange={(event) =>
														setSelectorInput(event.target.value)
													}
													placeholder={t(
														"pages.xray.balancer.selectorPlaceholder",
														"tag1, tag2",
													)}
													onKeyDown={(event) => {
														if (event.key === "Enter") {
															event.preventDefault();
															addSelectorTags(selectorInput);
															setSelectorInput("");
														}
													}}
												/>
												<Button
													size="xs"
													variant="outline"
													onClick={() => {
														addSelectorTags(selectorInput);
														setSelectorInput("");
													}}
												>
													{t("core.add", "Add")}
												</Button>
											</HStack>
											{selectorValue.length > 0 ? (
												<Wrap>
													{selectorValue.map((tag) => (
														<WrapItem key={tag}>
															<Tag size="sm" colorScheme="blue">
																<TagLabel>{tag}</TagLabel>
																<TagCloseButton
																	onClick={() => removeSelectorTag(tag)}
																/>
															</Tag>
														</WrapItem>
													))}
												</Wrap>
											) : (
												<Box>
													<Text fontSize="sm" color="gray.500">
														{t(
															"pages.xray.balancer.selectorHint",
															"Choose outbound tags or add custom tags.",
														)}
													</Text>
												</Box>
											)}
										</VStack>
										{emptySelector && (
											<FormErrorMessage>
												{t("pages.xray.balancer.selectorError")}
											</FormErrorMessage>
										)}
									</FormControl>
									<FormControl>
										<FormLabel>
											{t("pages.xray.balancer.fallbackTag")}
										</FormLabel>
										<SearchableTagSelect
											mode="single"
											options={selectableOutboundTags}
											value={fallbackTagValue}
											onChange={(value) =>
												modalForm.setValue("fallbackTag", value as string, {
													shouldDirty: true,
												})
											}
											placeholder={t("core.none", "None")}
											searchPlaceholder={t("search", "Search")}
											emptyText={t(
												"pages.xray.outbound.empty",
												"No outbound found",
											)}
										/>
									</FormControl>
								</VStack>
							</XrayDialogSection>
						</VStack>
					</XrayModalBody>
					<XrayModalFooter justifyContent="flex-end">
						<Button variant="outline" onClick={onClose}>
							{t("cancel")}
						</Button>
						<Button
							type="submit"
							colorScheme="primary"
							size="sm"
							isDisabled={!isValid}
						>
							{mode === "edit"
								? t("pages.xray.balancer.editBalancer")
								: t("pages.xray.balancer.addBalancer")}
						</Button>
					</XrayModalFooter>
				</form>
			</XrayModalContent>
		</Modal>
	);
};
