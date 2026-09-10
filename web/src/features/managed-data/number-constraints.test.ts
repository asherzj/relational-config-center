import { expect, it } from "vitest";
import { numberConstraintError } from "./number-constraints";
it("checks range and step at exact decimal precision including signed and exponential values",()=>{
 const options={options:[],min:"9007199254740993.000001",max:"9007199254740993.000009",step:"0.000002"};
 expect(numberConstraintError("9007199254740993.000000",options)).toContain("不能小于");
 expect(numberConstraintError("9007199254740993.000010",options)).toContain("不能大于");
 expect(numberConstraintError("9007199254740993.000002",options)).toContain("步长");
 expect(numberConstraintError("9007199254740993.000003",options)).toBeUndefined();
 expect(numberConstraintError("-1.25",{options:[],min:"-2",step:"0.25"})).toBeUndefined();
 expect(numberConstraintError("1.25e-6",{options:[],step:"0.00000025"})).toBeUndefined();
 expect(numberConstraintError("1e999999",{options:[]})).toContain("有效数字");
});
it("supports schema-legal leading-dot constraints without skipping validation or throwing",()=>{
 expect(numberConstraintError(".6",{options:[],min:".5",step:".1"})).toBeUndefined();
 expect(numberConstraintError(".65",{options:[],min:".5",step:".1"})).toContain("步长");
 expect(numberConstraintError(".6",{options:[],min:".5",step:"1"})).toContain("步长");
 expect(numberConstraintError("-.4",{options:[],min:"-.5",max:".5",step:".1"})).toBeUndefined();
});
