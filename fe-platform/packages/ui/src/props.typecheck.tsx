import {
  Avatar,
  Button,
  Card,
  CardContent,
  CardFooter,
  CardHeader,
  CardTitle,
  Checkbox,
  Dialog,
  Drawer,
  Image,
  Input,
  PriceTag,
  Select,
  Table,
} from "./index";

// @ts-expect-error className would bypass tokens
export const button = <Button className="x" />;
// @ts-expect-error style would bypass tokens
export const buttonStyle = <Button style={{ color: "red" }} />;
// @ts-expect-error className would bypass tokens
export const input = <Input label="a" className="x" />;
// @ts-expect-error className would bypass tokens
export const checkbox = <Checkbox label="a" className="x" />;
// @ts-expect-error className would bypass tokens
export const select = <Select label="a" options={[]} className="x" />;
const dialogBase = { triggerLabel: "a", title: "a", description: "a" };
// @ts-expect-error className would bypass tokens
export const dialog = <Dialog {...dialogBase} className="x" />;
// @ts-expect-error className would bypass tokens
export const table = <Table caption="a" columns={[]} rows={[]} className="x" />;
// @ts-expect-error className would bypass tokens
export const card = <Card className="x" />;
// @ts-expect-error style would bypass tokens
export const cardStyle = <Card style={{ color: "red" }} />;
// @ts-expect-error className would bypass tokens
export const cardHeader = <CardHeader className="x" />;
// @ts-expect-error className would bypass tokens
export const cardTitle = <CardTitle className="x" />;
// @ts-expect-error className would bypass tokens
export const cardContent = <CardContent className="x" />;
// @ts-expect-error className would bypass tokens
export const cardFooter = <CardFooter className="x" />;
// @ts-expect-error unknown variant
export const buttonVariant = <Button variant="outline" />;
// @ts-expect-error className would bypass tokens
export const priceTag = <PriceTag price={1} className="x" />;
// @ts-expect-error className would bypass tokens
export const image = <Image src="" alt="" aspect="square" className="x" />;
// @ts-expect-error className would bypass tokens
export const avatar = <Avatar className="x" />;
// @ts-expect-error className would bypass tokens
export const drawer = <Drawer open onClose={() => {}} className="x" />;
