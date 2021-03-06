/*
 * This service contains functions which can convert expression
 * like `{probbility: {>0.05}}` to `{$match: {probability: {$gt: 0.05}}}`
 * This service contains different workflows for different column types
 * The flow is following:
 * 1. preprocess - basic preprocessing (e.g. string trimming)
 * 2. Tokenization - divides input onto pieces, e.g. `>0.5` to '>' and '0.5'
 * 3. Parsing - applies and checks syntax rules.Produces internal
 *              representation of the expression
 * 4. Compile Predicate - compiles internal representation to MDB query bit
 * 5. Compile - compile MDB query bits to single $match expression
 */
mciModule.factory('MDBQueryAdaptor', function() {
  var mdbOpMapping = {
    '>': '$gt',
    '>=': '$gte',
    '<': '$lt',
    '<=': '$lte',
    '==': '$eq',
    'contains': '$regex',
    'icontains': function(term) {
      return {$regex: term, $options: 'i'}
    },
  }

  // Removes all whitespace symbols
  function condense(query) {
    return query.replace(/\s/g, '')
  }

  function numTypeTokenizer(query) {
    return query
      .match(/([<>]?)(=?)(.+)/)
      .slice(1, 4)
      .filter(_.identity)
  }

  function numTypeParser(tokens) {
    var t = tokens // shorthand
    var op

    // if the first token is either < either >
    if (_.contains('<>', t[0])) {
      if (t.length == 1) return // Parsing error
      op = t[0]
      if (t[1] == '=') op += t[1]
    }

    var term = +_.last(t)
    if (!term) return // Number parsing error

    return {
      op: op || '==',
      term: term,
    }
  }

  function strTypeTokenizer(query) { return [query] }

  function strTypeParser(tokens) {
    return {
      op: 'icontains',
      term: tokens[0],
    }
  }

  // field -> predicate -> MDBQuery?
  // :rtype: MDBQuery?
  function predicateCompiler(field) {
    return function(predicate) {
      if (!predicate) return // Invalid predicate
      var mdbOp = mdbOpMapping[predicate.op]
      if (!mdbOp) return // Invalid operation

      var outer = {}
      var inner = {}
      if(_.isFunction(mdbOp)) {
        inner = mdbOp(predicate.term)
      } else {
        inner[mdbOp] = predicate.term
      }
      outer[field] = inner
      return outer
    }
  }

  function compileFiltering(options) {
    var matchers = _.chain(options)
      .map(function(option) {
        var p = filterProcessor[option.type]
        // compile term expression
        return _.compose(
          p.compile(option.field),
          p.parse,
          p.tokenize,
          p.preprocess
        )(option.term)
      })
      .reduce(function(m, d) {
        return d ? _.extend(m, d) : m
      }, {}) // convert to obj
      .value()

    return !_.isEmpty(matchers) ? {$match: matchers} : undefined
  }

  var filterProcessor = {
    'string': {
      preprocess: condense,
      tokenize: strTypeTokenizer,
      parse: strTypeParser,
      compile: predicateCompiler,
    },
    'number': {
      preprocess: condense,
      tokenize: numTypeTokenizer,
      parse: numTypeParser,
      compile: predicateCompiler,
    },
  }

  // Creates aggregation entry for sorting
  // :param sort: input sorting options
  // :ptype sort: {field: '%field%', direction: 'asc|desc'}
  // :returns: mdb aggregation sorting entry
  // :rtype: {$sort: { ... }}
  function compileSorting(sort) {
    var q = {$sort: {}}
    if (!_.isEmpty(sort)) {
      q.$sort[sort.field] = sort.direction == 'asc' ? 1 : -1
    }
    return q
  }

  return {
    compileFiltering: compileFiltering,
    compileSorting: compileSorting,
    // For testing
    _condense: condense,
    _numTypeTokenizer: numTypeTokenizer,
    _strTypeTokenizer: strTypeTokenizer,
    _numTypeParser: numTypeParser,
    _strTypeParser: strTypeParser,
    _predicateCompiler: predicateCompiler,
  }
})
