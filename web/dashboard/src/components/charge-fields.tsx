import type { ChargeRule, ChargeSchedule, ChargeType } from "momobase";

import { ButtonGroup } from "@/components/ui/button-group";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from "@/components/ui/select";

export const zeroCharges: ChargeSchedule = {
	collection: { type: "flat", value: 0 },
	disbursement: { type: "flat", value: 0 },
};

function RuleField({
	id,
	label,
	rule,
	onChange,
}: {
	id: string;
	label: string;
	rule: ChargeRule;
	onChange: (rule: ChargeRule) => void;
}) {
	return (
		<div className="grid gap-2">
			<Label htmlFor={`${id}-value`}>{label}</Label>
			<ButtonGroup className="w-full">
				<Select
					value={rule.type}
					onValueChange={(type) =>
						onChange({
							...rule,
							type: (type ?? "flat") as ChargeType,
						})
					}
				>
					<SelectTrigger
						id={`${id}-type`}
						aria-label={`${label} charge type`}
					>
						<SelectValue />
					</SelectTrigger>
					<SelectContent>
						<SelectItem value="flat">Flat</SelectItem>
						<SelectItem value="percentage">Percentage</SelectItem>
					</SelectContent>
				</Select>
				<Input
					id={`${id}-value`}
					type="number"
					min={0}
					step={1}
					max={rule.type === "percentage" ? 10_000 : undefined}
					value={rule.value}
					onChange={(event) =>
						onChange({
							...rule,
							value: Math.max(
								0,
								Math.trunc(Number(event.target.value) || 0),
							),
						})
					}
				/>
			</ButtonGroup>
		</div>
	);
}

/** ChargeFields edits the complete per-service schedule expected by the API. */
export function ChargeFields({
	id,
	value,
	onChange,
}: {
	id: string;
	value: ChargeSchedule;
	onChange: (value: ChargeSchedule) => void;
}) {
	return (
		<div className="flex flex-col gap-2">
			<div>
				<p className="font-medium">Transaction charges</p>
				<p className="text-muted-foreground">
					Flat values use minor units; percentages use basis points
					(1,000 = 10%).
				</p>
			</div>
			<RuleField
				id={`${id}-collection`}
				label="Collections"
				rule={value.collection}
				onChange={(collection) => onChange({ ...value, collection })}
			/>
			<RuleField
				id={`${id}-disbursement`}
				label="Disbursements"
				rule={value.disbursement}
				onChange={(disbursement) =>
					onChange({ ...value, disbursement })
				}
			/>
		</div>
	);
}
