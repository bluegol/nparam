package nparamcli

import (
	"errors"
)

var (
	ErrConfigFile      error

	ErrCannotReadYaml  error
	ErrCannotWriteYaml error
	ErrCannotParseOpts error

	ErrCannotOpen      error
	ErrCannotReadXlsx  error
	ErrCannotCreate    error
	ErrCannotWrite     error


	ErrInvalidOptSpec           error
	ErrCannotUseTypeOrOpt error
	ErrUnknownTableOpt          error
	ErrUnknownFieldOpt          error
	ErrNoFieldType              error
	ErrFieldTypeRedefinition    error
	ErrFieldInconsistentRange   error

	ErrInvalidFieldName         error
	ErrDuplicateFieldNames      error


	ErrSymbolAlreadyDefined   error
	ErrNoSuchTable            error
	ErrCircularTableReference error

	ErrSymbolNotInRange       error
	ErrInvalidIntValue        error
	ErrInvalidFixed4Value     error
	ErrUnknownUnit error
)

func init() {
	ErrConfigFile = errors.New("컨피그 파일 오픈 실패함")

	ErrCannotReadYaml = errors.New("yaml 읽기 실패함")
	ErrCannotWriteYaml = errors.New("yaml 작성 실패함")
	ErrCannotParseOpts = errors.New("option 파싱 실패")

	ErrCannotOpen = errors.New("파일 오픈 실패함")
	ErrCannotReadXlsx = errors.New("엑셀 파일을 읽을 수 없음")
	ErrCannotCreate = errors.New("파일 생성 실패함")
	ErrCannotWrite = errors.New("파일 쓰기 실패함")



	ErrInvalidOptSpec = errors.New("옵션 지정이 잘못됨")
	ErrCannotUseTypeOrOpt = errors.New("그 상황에서 사용할 수 없는 타입 혹은 옵션임")
	ErrUnknownTableOpt = errors.New("테이블 옵션이 아님")
	ErrUnknownFieldOpt = errors.New("필드 옵션이 아님")
	ErrNoFieldType = errors.New("필드 타입이 지정되지 않음")
	ErrFieldTypeRedefinition = errors.New("필드 타입을 다시 정의하려 함")
	ErrFieldInconsistentRange = errors.New("필드 값의 범위들의 타입이 일관되지 않음")

	ErrInvalidFieldName = errors.New("필드 이름이 잘못됨")
	ErrDuplicateFieldNames = errors.New("필드 이름이 겹침")


	ErrSymbolAlreadyDefined = errors.New("심볼을 재정의하려함")
	ErrNoSuchTable = errors.New("그런 테이블 없다")
	ErrCircularTableReference = errors.New("테이블 순환 참조 발생")

	ErrSymbolNotInRange = errors.New("심볼이 정해진 범위에 있지 않음")
	ErrInvalidIntValue = errors.New("값이 정수가 아님")
	ErrInvalidFixed4Value = errors.New("값이 fixed4가 아님")
	ErrUnknownUnit = errors.New("단위를 알 수 없음")
}
