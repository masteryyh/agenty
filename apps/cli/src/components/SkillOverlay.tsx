/*
Copyright © 2026 masteryyh <yyh991013@163.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

	http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

import { useEffect, useRef, useState } from "react";
import { useInput } from "../hooks/useInput";
import { Box, Select, Spinner, Text } from "./ui";
import { useAppStore } from "../state/store";
import type { SkillDto } from "../api/types";

function truncate(text: string, max: number): string {
	const single = text.replace(/\s+/g, " ").trim();
	if (single.length <= max) return single;
	return `${single.slice(0, max)}…`;
}

export function SkillOverlay() {
	const client = useAppStore((s) => s.client);
	const session = useAppStore((s) => s.session);
	const setOverlay = useAppStore((s) => s.setOverlay);

	const [skills, setSkills] = useState<SkillDto[] | null>(null);
	const [error, setError] = useState<string | null>(null);
	const [selectedSkill, setSelectedSkill] = useState<SkillDto | null>(null);
	const [content, setContent] = useState<string | null>(null);
	const [contentLoading, setContentLoading] = useState(false);
	const contentRef = useRef<SkillDto | null>(null);
	contentRef.current = selectedSkill;

	useEffect(() => {
		if (!client) return;
		let cancelled = false;
		client
			.listSkills(session?.id)
			.then((r) => {
				if (!cancelled) setSkills(r);
			})
			.catch((e) => {
				if (!cancelled) setError((e as Error).message);
			});
		return () => {
			cancelled = true;
		};
	}, [client, session?.id]);

	useInput((_input, key) => {
		if (key.escape) {
			if (selectedSkill) {
				setSelectedSkill(null);
				setContent(null);
			} else {
				setOverlay(null);
			}
		}
	});

	// Initial loading
	if (error) {
		return (
			<Box flexDirection="column" paddingX={2} paddingY={1}>
				<Text color="red">Failed to load skills: {error}</Text>
				<Text dimColor>Press Esc to close.</Text>
			</Box>
		);
	}

	if (skills === null) {
		return (
			<Box paddingX={2} paddingY={1}>
				<Spinner label="Loading skills..." />
			</Box>
		);
	}

	// Skill content view
	if (selectedSkill) {
		return (
			<Box flexDirection="column" paddingX={2} paddingY={1}>
				<Box marginBottom={1} gap={1}>
					<Text color="magenta" bold>
						{selectedSkill.name}
					</Text>
					<Text dimColor>
						{selectedSkill.scope === "global" ? "[G]" : "[P]"} · Esc to go back
					</Text>
				</Box>
				{selectedSkill.description ? (
					<Box marginBottom={1}>
						<Text color="yellow">{selectedSkill.description}</Text>
					</Box>
				) : null}
				{contentLoading ? (
					<Spinner label="Loading content..." />
				) : content !== null ? (
					<Box flexDirection="column">
						{content ? (
							<Text>{content}</Text>
						) : (
							<Text dimColor>Empty skill file.</Text>
						)}
					</Box>
				) : (
					<Text dimColor>Failed to load content.</Text>
				)}
			</Box>
		);
	}

	// Skill list view
	if (skills.length === 0) {
		return (
			<Box flexDirection="column" paddingX={2} paddingY={1}>
				<Box marginBottom={1}>
					<Text color="magenta" bold>
						Skills
					</Text>
					<Text dimColor> · Esc to close</Text>
				</Box>
				<Text dimColor>No skills found.</Text>
			</Box>
		);
	}

	const handleSelect = async (idx: number) => {
		const skill = skills[idx];
		if (!skill || !client) return;
		setSelectedSkill(skill);
		setContent(null);
		setContentLoading(true);
		try {
			const r = await client.getSkillContent(skill.name, session?.id);
			setContent(r.content);
		} catch {
			setContent(null);
		} finally {
			setContentLoading(false);
		}
	};

	const options = skills.map((s, i) => {
		const scopeTag =
			s.scope === "project" ? "[P]" : s.scope === "global" ? "[G]" : "";
		const desc = s.description ? ` — ${truncate(s.description, 60)}` : "";
		return {
			label: `${scopeTag} ${s.name}${desc}`,
			value: String(i),
		};
	});

	return (
		<Box flexDirection="column" paddingX={2} paddingY={1}>
			<Box marginBottom={1} gap={1}>
				<Text color="magenta" bold>
					Skills
				</Text>
				<Text dimColor>· Esc to close</Text>
			</Box>
			<Select
				options={options}
				visibleOptionCount={Math.min(options.length, 10)}
				onChange={async (v) => {
					const idx = Number(v);
					await handleSelect(idx);
				}}
			/>
		</Box>
	);
}
