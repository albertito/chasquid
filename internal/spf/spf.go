// Package spf implements SPF (Sender Policy Framework) lookup and validation.
//
// Supported:
//  - "all".
//  - "include".
//  - "a".
//  - "mx".
//  - "ip4".
//  - "ip6".
//  - "redirect".
//
// Not supported (return Neutral if used):
//  - "exists".
//  - "ptr".
//  - "exp".
//  - Macros.
//
// References:
// https://tools.ietf.org/html/rfc7208
// https://en.wikipedia.org/wiki/Sender_Policy_Framework
package spf

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

// TODO: Neutral if not supported (including macros).

// Functions that we can override for testing purposes.
var (
	lookupTXT func(domain string) (txts []string, err error) = net.LookupTXT
	lookupMX  func(domain string) (mxs []*net.MX, err error) = net.LookupMX
	lookupIP  func(host string) (ips []net.IP, err error)    = net.LookupIP
)

// Results and Errors. Note the values have meaning, we use them in headers.
// https://tools.ietf.org/html/rfc7208#section-8
type Result string

var (
	// https://tools.ietf.org/html/rfc7208#section-8.1
	// Not able to reach any conclusion.
	None = Result("none")

	// https://tools.ietf.org/html/rfc7208#section-8.2
	// No definite assertion (positive or negative).
	Neutral = Result("neutral")

	// https://tools.ietf.org/html/rfc7208#section-8.3
	// Client is authorized to inject mail.
	Pass = Result("pass")

	// https://tools.ietf.org/html/rfc7208#section-8.4
	// Client is *not* authorized to use the domain
	Fail = Result("fail")

	// https://tools.ietf.org/html/rfc7208#section-8.5
	// Not authorized, but unwilling to make a strong policy statement/
	SoftFail = Result("softfail")

	// https://tools.ietf.org/html/rfc7208#section-8.6
	// Transient error while performing the check.
	TempError = Result("temperror")

	// https://tools.ietf.org/html/rfc7208#section-8.7
	// Records could not be correctly interpreted.
	PermError = Result("permerror")
)

var QualToResult = map[byte]Result{
	'+': Pass,
	'-': Fail,
	'~': SoftFail,
	'?': Neutral,
}

// CheckHost function fetches SPF records, parses them, and evaluates them to
// determine whether a particular host is or is not permitted to send mail
// with a given identity.
// Reference: https://tools.ietf.org/html/rfc7208#section-4
func CheckHost(ip net.IP, domain string) (Result, error) {
	r := &resolution{ip, 0}
	return r.Check(domain)
}

type resolution struct {
	ip    net.IP
	count uint
}

func (r *resolution) Check(domain string) (Result, error) {
	// Limit the number of resolutions to 10
	// https://tools.ietf.org/html/rfc7208#section-4.6.4
	if r.count > 10 {
		return PermError, fmt.Errorf("lookup limit reached")
	}
	r.count++

	txt, err := getDNSRecord(domain)
	if err != nil {
		if isTemporary(err) {
			return TempError, err
		}
		// Could not resolve the name, it may be missing the record.
		// https://tools.ietf.org/html/rfc7208#section-2.6.1
		return None, err
	}

	if txt == "" {
		// No record => None.
		// https://tools.ietf.org/html/rfc7208#section-4.6
		return None, nil
	}

	fields := strings.Fields(txt)

	// redirects must be handled after the rest; instead of having two loops,
	// we just move them to the end.
	var newfields, redirects []string
	for _, field := range fields {
		if strings.HasPrefix(field, "redirect:") {
			redirects = append(redirects, field)
		} else {
			newfields = append(newfields, field)
		}
	}
	fields = append(newfields, redirects...)

	for _, field := range fields {
		if strings.HasPrefix(field, "v=") {
			continue
		}
		if r.count > 10 {
			return PermError, fmt.Errorf("lookup limit reached")
		}
		if strings.Contains(field, "%") {
			return Neutral, fmt.Errorf("macros not supported")
		}

		// See if we have a qualifier, defaulting to + (pass).
		// https://tools.ietf.org/html/rfc7208#section-4.6.2
		result, ok := QualToResult[field[0]]
		if ok {
			field = field[1:]
		} else {
			result = Pass
		}

		if field == "all" {
			// https://tools.ietf.org/html/rfc7208#section-5.1
			return result, fmt.Errorf("matched 'all'")
		} else if strings.HasPrefix(field, "include:") {
			if ok, res, err := r.includeField(result, field); ok {
				return res, err
			}
		} else if strings.HasPrefix(field, "a") {
			if ok, res, err := r.aField(result, field, domain); ok {
				return res, err
			}
		} else if strings.HasPrefix(field, "mx") {
			if ok, res, err := r.mxField(result, field, domain); ok {
				return res, err
			}
		} else if strings.HasPrefix(field, "ip4:") || strings.HasPrefix(field, "ip6:") {
			if ok, res, err := r.ipField(result, field); ok {
				return res, err
			}
		} else if strings.HasPrefix(field, "exists") {
			return Neutral, fmt.Errorf("'exists' not supported")
		} else if strings.HasPrefix(field, "ptr") {
			return Neutral, fmt.Errorf("'ptr' not supported")
		} else if strings.HasPrefix(field, "exp=") {
			return Neutral, fmt.Errorf("'exp' not supported")
		} else if strings.HasPrefix(field, "redirect=") {
			// https://tools.ietf.org/html/rfc7208#section-6.1
			result, err := r.Check(field[len("redirect="):])
			if result == None {
				result = PermError
			}
			return result, err
		} else {
			// http://www.openspf.org/SPF_Record_Syntax
			return PermError, fmt.Errorf("unknown field %q", field)
		}
	}

	// Got to the end of the evaluation without a result => Neutral.
	// https://tools.ietf.org/html/rfc7208#section-4.7
	return Neutral, nil
}

// getDNSRecord gets TXT records from the given domain, and returns the SPF
// (if any).  Note that at most one SPF is allowed per a given domain:
// https://tools.ietf.org/html/rfc7208#section-3
// https://tools.ietf.org/html/rfc7208#section-3.2
// https://tools.ietf.org/html/rfc7208#section-4.5
func getDNSRecord(domain string) (string, error) {
	txts, err := lookupTXT(domain)
	if err != nil {
		return "", err
	}

	for _, txt := range txts {
		if strings.HasPrefix(txt, "v=spf1 ") {
			return txt, nil
		}

		// An empty record is explicitly allowed:
		// https://tools.ietf.org/html/rfc7208#section-4.5
		if txt == "v=spf1" {
			return txt, nil
		}
	}

	return "", nil
}

func isTemporary(err error) bool {
	derr, ok := err.(*net.DNSError)
	return ok && derr.Temporary()
}

// ipField processes an "ip" field.
func (r *resolution) ipField(res Result, field string) (bool, Result, error) {
	fip := field[4:]
	if strings.Contains(fip, "/") {
		_, ipnet, err := net.ParseCIDR(fip)
		if err != nil {
			return true, PermError, err
		}
		if ipnet.Contains(r.ip) {
			return true, res, fmt.Errorf("matched %v", ipnet)
		}
	} else {
		ip := net.ParseIP(fip)
		if ip == nil {
			return true, PermError, fmt.Errorf("invalid ipX value")
		}
		if ip.Equal(r.ip) {
			return true, res, fmt.Errorf("matched %v", ip)
		}
	}

	return false, "", nil
}

// includeField processes an "include" field.
func (r *resolution) includeField(res Result, field string) (bool, Result, error) {
	// https://tools.ietf.org/html/rfc7208#section-5.2
	incdomain := field[len("include:"):]
	ir, err := r.Check(incdomain)
	switch ir {
	case Pass:
		return true, res, err
	case Fail, SoftFail, Neutral:
		return false, ir, err
	case TempError:
		return true, TempError, err
	case PermError, None:
		return true, PermError, err
	}

	return false, "", fmt.Errorf("This should never be reached")

}

func ipMatch(ip, tomatch net.IP, mask int) (bool, error) {
	if mask >= 0 {
		_, ipnet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", tomatch.String(), mask))
		if err != nil {
			return false, err
		}
		if ipnet.Contains(ip) {
			return true, fmt.Errorf("%v", ipnet)
		}
		return false, nil
	} else {
		if ip.Equal(tomatch) {
			return true, fmt.Errorf("%v", tomatch)
		}
		return false, nil
	}
}

var aRegexp = regexp.MustCompile("a(:([^/]+))?(/(.+))?")
var mxRegexp = regexp.MustCompile("mx(:([^/]+))?(/(.+))?")

func domainAndMask(re *regexp.Regexp, field, domain string) (string, int, error) {
	var err error
	mask := -1
	if groups := re.FindStringSubmatch(field); groups != nil {
		if groups[2] != "" {
			domain = groups[2]
		}
		if groups[4] != "" {
			mask, err = strconv.Atoi(groups[4])
			if err != nil {
				return "", -1, fmt.Errorf("error parsing mask")
			}
		}
	}

	return domain, mask, nil
}

// aField processes an "a" field.
func (r *resolution) aField(res Result, field, domain string) (bool, Result, error) {
	// https://tools.ietf.org/html/rfc7208#section-5.3
	domain, mask, err := domainAndMask(aRegexp, field, domain)
	if err != nil {
		return true, PermError, err
	}

	r.count++
	ips, err := lookupIP(domain)
	if err != nil {
		// https://tools.ietf.org/html/rfc7208#section-5
		if isTemporary(err) {
			return true, TempError, err
		}
		return false, "", err
	}
	for _, ip := range ips {
		ok, err := ipMatch(r.ip, ip, mask)
		if ok {
			return true, res, fmt.Errorf("matched 'a' (%v)", err)
		} else if err != nil {
			return true, PermError, err
		}
	}

	return false, "", nil
}

// mxField processes an "mx" field.
func (r *resolution) mxField(res Result, field, domain string) (bool, Result, error) {
	// https://tools.ietf.org/html/rfc7208#section-5.4
	domain, mask, err := domainAndMask(mxRegexp, field, domain)
	if err != nil {
		return true, PermError, err
	}

	r.count++
	mxs, err := lookupMX(domain)
	if err != nil {
		// https://tools.ietf.org/html/rfc7208#section-5
		if isTemporary(err) {
			return true, TempError, err
		}
		return false, "", err
	}
	mxips := []net.IP{}
	for _, mx := range mxs {
		r.count++
		ips, err := lookupIP(mx.Host)
		if err != nil {
			// https://tools.ietf.org/html/rfc7208#section-5
			if isTemporary(err) {
				return true, TempError, err
			}
			return false, "", err
		}
		mxips = append(mxips, ips...)
	}
	for _, ip := range mxips {
		ok, err := ipMatch(r.ip, ip, mask)
		if ok {
			return true, res, fmt.Errorf("matched 'mx' (%v)", err)
		} else if err != nil {
			return true, PermError, err
		}
	}

	return false, "", nil
}
