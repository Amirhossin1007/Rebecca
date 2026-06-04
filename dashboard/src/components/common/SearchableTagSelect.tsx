import {
	Box,
	Button,
	chakra,
	HStack,
	Input,
	Menu,
	MenuButton,
	MenuItem,
	MenuList,
	Text,
	useColorModeValue,
	VStack,
} from "@chakra-ui/react";
import {
	CheckIcon,
	ChevronDownIcon,
	MagnifyingGlassIcon,
	XMarkIcon,
} from "@heroicons/react/24/outline";
import { type FC, type MouseEvent, useMemo, useState } from "react";

const Check = chakra(CheckIcon, { baseStyle: { w: 4, h: 4 } });
const ChevronDown = chakra(ChevronDownIcon, { baseStyle: { w: 4, h: 4 } });
const Search = chakra(MagnifyingGlassIcon, { baseStyle: { w: 4, h: 4 } });
const X = chakra(XMarkIcon, { baseStyle: { w: 3.5, h: 3.5 } });

type SearchableTagSelectProps = {
	emptyText?: string;
	mode?: "single" | "multiple";
	onChange: (value: string | string[]) => void;
	options: string[];
	placeholder: string;
	searchPlaceholder?: string;
	size?: "sm" | "md";
	value: string | string[];
};

export const SearchableTagSelect: FC<SearchableTagSelectProps> = ({
	emptyText = "No options found",
	mode = "single",
	onChange,
	options,
	placeholder,
	searchPlaceholder = "Search",
	size = "sm",
	value,
}) => {
	const [search, setSearch] = useState("");
	const selectedValues = useMemo(
		() =>
			new Set(
				Array.isArray(value)
					? value.filter(Boolean)
					: value
						? [value]
						: [],
			),
		[value],
	);
	const filteredOptions = useMemo(() => {
		const term = search.trim().toLowerCase();
		const sorted = [...options].sort((left, right) => {
			const leftSelected = selectedValues.has(left);
			const rightSelected = selectedValues.has(right);
			if (leftSelected === rightSelected) return left.localeCompare(right);
			return leftSelected ? -1 : 1;
		});
		if (!term) return sorted;
		return sorted.filter((option) => option.toLowerCase().includes(term));
	}, [options, search, selectedValues]);

	const borderColor = useColorModeValue("blackAlpha.200", "whiteAlpha.200");
	const selectedBg = useColorModeValue("primary.50", "whiteAlpha.100");
	const hoverBg = useColorModeValue("blackAlpha.50", "whiteAlpha.100");
	const mutedColor = useColorModeValue("gray.500", "gray.400");

	const selectedList = Array.from(selectedValues);
	const buttonText =
		mode === "multiple"
			? selectedList.length
				? selectedList.join(", ")
				: placeholder
			: selectedList[0] || placeholder;

	const updateValue = (option: string) => {
		if (mode === "single") {
			onChange(selectedValues.has(option) ? "" : option);
			return;
		}
		const next = new Set(selectedValues);
		if (next.has(option)) next.delete(option);
		else next.add(option);
		onChange(Array.from(next));
	};

	const removeValue = (option: string) => {
		if (mode === "single") {
			onChange("");
			return;
		}
		onChange(Array.from(selectedValues).filter((item) => item !== option));
	};

	return (
		<Menu closeOnSelect={false} isLazy placement="bottom-start">
			<MenuButton
				as={Button}
				rightIcon={<ChevronDown />}
				size={size}
				variant="outline"
				w="full"
				justifyContent="space-between"
				textAlign="start"
				fontWeight={selectedList.length ? "medium" : "normal"}
			>
				<Text as="span" noOfLines={1} color={selectedList.length ? undefined : mutedColor}>
					{buttonText}
				</Text>
			</MenuButton>
			<MenuList
				minW="260px"
				maxW="min(420px, calc(100vw - 32px))"
				maxH="320px"
				overflowY="auto"
				p={2}
				borderColor={borderColor}
			>
				<HStack spacing={2} mb={2}>
					<Search color={mutedColor} />
					<Input
						size="sm"
						value={search}
						onChange={(event) => setSearch(event.target.value)}
						placeholder={searchPlaceholder}
						autoFocus
					/>
				</HStack>
				<VStack align="stretch" spacing={1}>
					{filteredOptions.length === 0 ? (
						<Box px={3} py={2}>
							<Text fontSize="sm" color={mutedColor}>
								{emptyText}
							</Text>
						</Box>
					) : (
						filteredOptions.map((option) => {
							const selected = selectedValues.has(option);
							return (
								<MenuItem
									key={option}
									borderRadius="md"
									bg={selected ? selectedBg : "transparent"}
									_hover={{ bg: selected ? selectedBg : hoverBg }}
									onClick={() => updateValue(option)}
								>
									<HStack w="full" justifyContent="space-between" spacing={3}>
										<HStack minW={0} spacing={2}>
											<Box w="18px" color={selected ? "primary.500" : "transparent"}>
												<Check />
											</Box>
											<Text noOfLines={1}>{option}</Text>
										</HStack>
										{selected && (
											<Box
												as="span"
												role="button"
												aria-label={`Remove ${option}`}
												borderRadius="full"
												color="red.500"
												p={1}
												onClick={(event: MouseEvent<HTMLSpanElement>) => {
													event.preventDefault();
													event.stopPropagation();
													removeValue(option);
												}}
												_hover={{ bg: "red.50" }}
												_dark={{ _hover: { bg: "whiteAlpha.100" } }}
											>
												<X />
											</Box>
										)}
									</HStack>
								</MenuItem>
							);
						})
					)}
				</VStack>
			</MenuList>
		</Menu>
	);
};
